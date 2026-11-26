package objectstore

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// S3Config is how an S3 server is reached: its endpoint, its region, the
// bucket, and the access key the signature is made with.
type S3Config struct {
	Endpoint  string
	Region    string
	Bucket    string
	AccessKey string
	SecretKey string
	PathStyle bool
	Insecure  bool
	Timeout   time.Duration
}

// S3 is a bucket of an S3 server: AWS's, or one of the servers that speak
// its protocol — OSS, COS, OBS, Kodo, MinIO, RustFS, and the XML API of
// Google Cloud Storage.
type S3 struct {
	cfg    S3Config
	base   *url.URL
	client *http.Client
}

// NewS3 opens the bucket, signing with the key.
func NewS3(cfg S3Config) (*S3, error) {
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("no bucket")
	}
	u, err := parseEndpoint(cfg.Endpoint, cfg.Insecure)
	if err != nil {
		return nil, err
	}
	if cfg.Region == "" {
		cfg.Region = "us-east-1"
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &S3{cfg: cfg, base: u, client: &http.Client{Timeout: timeout}}, nil
}

// parseEndpoint reads an endpoint as a URL, https unless it says otherwise.
func parseEndpoint(endpoint string, insecure bool) (*url.URL, error) {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = "s3.amazonaws.com"
	}
	if !strings.Contains(endpoint, "://") {
		scheme := "https://"
		if insecure {
			scheme = "http://"
		}
		endpoint = scheme + endpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	if u.Host == "" {
		return nil, fmt.Errorf("endpoint %q has no host", endpoint)
	}
	return u, nil
}

// Kind is the storage's name.
func (s *S3) Kind() string { return "S3" }

// Home is the root of the bucket.
func (s *S3) Home() string { return "" }

// List reads what the prefix holds, a level of it: the folders its keys
// share, and the objects directly under it.
func (s *S3) List(ctx context.Context, dir string) ([]Entry, error) {
	prefix := CleanKey(dir)
	if prefix != "" {
		prefix += "/"
	}
	var entries []Entry
	token := ""
	for {
		q := url.Values{"list-type": {"2"}, "delimiter": {"/"}, "prefix": {prefix}}
		if token != "" {
			q.Set("continuation-token", token)
		}
		resp, err := s.do(ctx, http.MethodGet, "", q, nil, emptySHA, nil)
		if err != nil {
			return nil, err
		}
		var out listResult
		err = xml.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, c := range out.Contents {
			if strings.HasSuffix(c.Key, "/") {
				continue // the placeholder of a folder, which the prefixes hold
			}
			key := CleanKey(c.Key)
			entries = append(entries, Entry{
				Key: key, Name: BaseKey(key), Size: c.Size, ModTime: c.LastModified,
			})
		}
		for _, p := range out.Prefixes {
			key := CleanKey(p.Prefix)
			if key == "" {
				continue
			}
			entries = append(entries, Entry{Key: key, Name: BaseKey(key), Dir: true})
		}
		if !out.IsTruncated || out.NextToken == "" {
			break
		}
		token = out.NextToken
	}
	return entries, nil
}

// listResult is the XML of a listing, the parts the panel needs.
type listResult struct {
	Contents []struct {
		Key          string    `xml:"Key"`
		LastModified time.Time `xml:"LastModified"`
		Size         int64     `xml:"Size"`
		Class        string    `xml:"StorageClass"`
	} `xml:"Contents"`
	Prefixes []struct {
		Prefix string `xml:"Prefix"`
	} `xml:"CommonPrefixes"`
	IsTruncated bool   `xml:"IsTruncated"`
	NextToken   string `xml:"NextContinuationToken"`
}

// Get opens an object.
func (s *S3) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := s.do(ctx, http.MethodGet, key, nil, nil, emptySHA, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Put writes an object, of the bytes r holds from its start.
func (s *S3) Put(ctx context.Context, key string, r io.ReadSeeker, size int64, contentType string, progress Progress) error {
	return s.put(ctx, CleanKey(key), r, size, contentType, progress)
}

// put writes the key as it is: a folder's keeps the slash at its end.
func (s *S3) put(ctx context.Context, key string, r io.ReadSeeker, size int64, contentType string, progress Progress) error {
	sum := sha256.New()
	if _, err := io.Copy(sum, r); err != nil {
		return err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return err
	}
	headers := map[string]string{"Content-Length": fmt.Sprint(size)}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	var body io.Reader = r
	if progress != nil {
		body = &countingReader{r: r, report: progress}
	}
	resp, err := s.do(ctx, http.MethodPut, key, nil, headers, hex.EncodeToString(sum.Sum(nil)), body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Delete removes an object, or every key under a folder.
func (s *S3) Delete(ctx context.Context, key string, dir bool) error {
	key = CleanKey(key)
	if !dir {
		resp, err := s.do(ctx, http.MethodDelete, key, nil, nil, emptySHA, nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}
	entries, err := s.List(ctx, key)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := s.Delete(ctx, e.Key, e.Dir); err != nil {
			return err
		}
	}
	return nil
}

// Mkdir makes a folder the way the consoles do: a key of no bytes, ending
// with the slash, which the listing shows as a prefix.
func (s *S3) Mkdir(ctx context.Context, key string) error {
	return s.put(ctx, CleanKey(key)+"/", strings.NewReader(""), 0, "", nil)
}

// Stat reads one object's size and time.
func (s *S3) Stat(ctx context.Context, key string) (Entry, error) {
	resp, err := s.do(ctx, http.MethodHead, key, nil, nil, emptySHA, nil)
	if err != nil {
		return Entry{}, err
	}
	defer resp.Body.Close()
	e := Entry{Key: CleanKey(key), Name: BaseKey(key), Size: resp.ContentLength}
	if t, err := http.ParseTime(resp.Header.Get("Last-Modified")); err == nil {
		e.ModTime = t
	}
	return e, nil
}

const emptySHA = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

// Probe asks the bucket for its root, as a HEAD of it.
func (s *S3) Probe(ctx context.Context) error {
	resp, err := s.do(ctx, http.MethodHead, "", nil, nil, emptySHA, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// do sends a request for key, signed, with query, and returns the response
// of a status that worked.
func (s *S3) do(ctx context.Context, method, key string, query url.Values, headers map[string]string, payloadHash string, body io.Reader) (*http.Response, error) {
	req, err := s.request(method, key, query, payloadHash, body)
	if err != nil {
		return nil, err
	}
	for k, v := range headers {
		if strings.EqualFold(k, "Content-Length") {
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				req.ContentLength = n
			}
			continue
		}
		req.Header.Set(k, v)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return resp, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
	return nil, &StatusError{Op: method, Key: key, Status: resp.StatusCode, Body: string(msg)}
}

// request builds the URL of key and signs it.
func (s *S3) request(method, key string, query url.Values, payloadHash string, body io.Reader) (*http.Request, error) {
	u := *s.base
	prefix := strings.TrimSuffix(u.Path, "/")
	var decoded, encoded string
	if s.cfg.PathStyle {
		decoded = prefix + "/" + s.cfg.Bucket + "/" + key
		encoded = prefix + "/" + uriEncode(s.cfg.Bucket, false) + "/" + uriEncode(key, true)
	} else {
		u.Host = s.cfg.Bucket + "." + u.Host
		decoded = prefix + "/" + key
		encoded = prefix + "/" + uriEncode(key, true)
	}
	u.Path, u.RawPath = decoded, encoded
	if query != nil {
		u.RawQuery = encodeQuery(query)
	}
	req, err := http.NewRequest(method, u.String(), body)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", payloadHash)
	req.Header.Set("Host", u.Host)
	s.sign(req, amzDate, payloadHash, now)
	return req, nil
}

// sign writes the Authorization of a request, with the version 4 signature
// S3 asks for.
func (s *S3) sign(req *http.Request, amzDate, payloadHash string, now time.Time) {
	day := now.Format("20060102")
	scope := strings.Join([]string{day, s.cfg.Region, "s3", "aws4_request"}, "/")
	canonical := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.RawQuery,
		"host:" + req.URL.Host + "\n" +
			"x-amz-content-sha256:" + payloadHash + "\n" +
			"x-amz-date:" + amzDate + "\n",
		"host;x-amz-content-sha256;x-amz-date",
		payloadHash,
	}, "\n")
	sum := sha256.Sum256([]byte(canonical))
	toSign := strings.Join([]string{
		"AWS4-HMAC-SHA256", amzDate, scope, hex.EncodeToString(sum[:]),
	}, "\n")
	key := hmacSum([]byte("AWS4"+s.cfg.SecretKey), day)
	key = hmacSum(key, s.cfg.Region)
	key = hmacSum(key, "s3")
	key = hmacSum(key, "aws4_request")
	signature := hex.EncodeToString(hmacSum(key, toSign))
	req.Header.Set("Authorization", fmt.Sprintf(
		"AWS4-HMAC-SHA256 Credential=%s/%s, SignedHeaders=host;x-amz-content-sha256;x-amz-date, Signature=%s",
		s.cfg.AccessKey, scope, signature))
}

// hmacSum is the HMAC of data with key.
func hmacSum(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// uriEncode escapes s as S3 wants it: the unreserved characters stay, and
// the slash does too when keepSlash says so.
func uriEncode(s string, keepSlash bool) string {
	var b bytes.Buffer
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'A' <= c && c <= 'Z', 'a' <= c && c <= 'z', '0' <= c && c <= '9',
			c == '-', c == '.', c == '_', c == '~':
			b.WriteByte(c)
		case c == '/' && keepSlash:
			b.WriteByte(c)
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

// encodeQuery sorts the query and escapes it as S3 wants it.
func encodeQuery(q url.Values) string {
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		for _, v := range q[k] {
			parts = append(parts, uriEncode(k, false)+"="+uriEncode(v, false))
		}
	}
	return strings.Join(parts, "&")
}
