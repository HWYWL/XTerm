package objectstore

import (
	"context"
	"encoding/base64"
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

// AzureConfig is how a container of Azure Blob is reached: the account's
// endpoint, the container, and the key the signature is made with.
type AzureConfig struct {
	Endpoint  string
	Account   string
	Container string
	AccessKey string
	Timeout   time.Duration
}

// Azure is a container of Azure Blob storage.
type Azure struct {
	cfg    AzureConfig
	base   *url.URL
	client *http.Client
}

// NewAzure opens the container.
func NewAzure(cfg AzureConfig) (*Azure, error) {
	if strings.TrimSpace(cfg.Container) == "" {
		return nil, fmt.Errorf("no container")
	}
	u, err := parseEndpoint(cfg.Endpoint, false)
	if err != nil {
		return nil, err
	}
	cfg.Account = strings.TrimSpace(cfg.Account)
	if cfg.Account == "" {
		cfg.Account = strings.Split(u.Host, ".")[0]
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &Azure{cfg: cfg, base: u, client: &http.Client{Timeout: timeout}}, nil
}

// Kind is the storage's name.
func (a *Azure) Kind() string { return "Azure Blob" }

// Home is the container's root.
func (a *Azure) Home() string { return "" }

// List reads what the prefix holds, a level of it.
func (a *Azure) List(ctx context.Context, dir string) ([]Entry, error) {
	prefix := CleanKey(dir)
	if prefix != "" {
		prefix += "/"
	}
	var entries []Entry
	marker := ""
	for {
		q := url.Values{"restype": {"container"}, "comp": {"list"}, "delimiter": {"/"}}
		if prefix != "" {
			q.Set("prefix", prefix)
		}
		if marker != "" {
			q.Set("marker", marker)
		}
		resp, err := a.do(ctx, http.MethodGet, "", q, nil, nil)
		if err != nil {
			return nil, err
		}
		var out azureList
		err = xml.NewDecoder(resp.Body).Decode(&out)
		resp.Body.Close()
		if err != nil {
			return nil, err
		}
		for _, b := range out.Blobs.Blobs {
			key := CleanKey(b.Name)
			entries = append(entries, Entry{
				Key: key, Name: BaseKey(key), Size: b.Properties.Length, ModTime: b.Properties.Modified,
			})
		}
		for _, p := range out.Blobs.Prefixes {
			key := CleanKey(p.Name)
			if key == "" {
				continue
			}
			entries = append(entries, Entry{Key: key, Name: BaseKey(key), Dir: true})
		}
		if out.NextMarker == "" {
			break
		}
		marker = out.NextMarker
	}
	return entries, nil
}

// azureList is the XML of a container's listing.
type azureList struct {
	Blobs struct {
		Blobs []struct {
			Name       string `xml:"Name"`
			Properties struct {
				Modified time.Time `xml:"Last-Modified"`
				Length   int64     `xml:"Content-Length"`
				Tier     string    `xml:"AccessTier"`
			} `xml:"Properties"`
		} `xml:"Blob"`
		Prefixes []struct {
			Name string `xml:"Name"`
		} `xml:"BlobPrefix"`
	} `xml:"Blobs"`
	NextMarker string `xml:"NextMarker"`
}

// Get opens a blob.
func (a *Azure) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	resp, err := a.do(ctx, http.MethodGet, key, nil, nil, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// Put writes a blob.
func (a *Azure) Put(ctx context.Context, key string, r io.ReadSeeker, size int64, contentType string, progress Progress) error {
	return a.put(ctx, CleanKey(key), r, size, contentType, progress)
}

// put writes the key as it is: a folder's keeps the slash at its end.
func (a *Azure) put(ctx context.Context, key string, r io.ReadSeeker, size int64, contentType string, progress Progress) error {
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	headers := map[string]string{
		"x-ms-blob-type": "BlockBlob",
		"Content-Type":   contentType,
		"Content-Length": fmt.Sprint(size),
	}
	var body io.Reader = r
	if progress != nil {
		body = &countingReader{r: r, report: progress}
	}
	resp, err := a.do(ctx, http.MethodPut, key, nil, headers, body)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// Delete removes a blob, or every blob under a folder.
func (a *Azure) Delete(ctx context.Context, key string, dir bool) error {
	key = CleanKey(key)
	if !dir {
		resp, err := a.do(ctx, http.MethodDelete, key, nil, nil, nil)
		if err != nil {
			return err
		}
		resp.Body.Close()
		return nil
	}
	entries, err := a.List(ctx, key)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := a.Delete(ctx, e.Key, e.Dir); err != nil {
			return err
		}
	}
	return nil
}

// Mkdir makes a folder the way the consoles do: a blob of no bytes, ending
// with the slash, which the listing shows as a prefix.
func (a *Azure) Mkdir(ctx context.Context, key string) error {
	return a.put(ctx, CleanKey(key)+"/", strings.NewReader(""), 0, "", nil)
}

// Stat reads one blob's size and time.
func (a *Azure) Stat(ctx context.Context, key string) (Entry, error) {
	resp, err := a.do(ctx, http.MethodHead, key, nil, nil, nil)
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

// Probe asks the container for its root.
func (a *Azure) Probe(ctx context.Context) error {
	resp, err := a.do(ctx, http.MethodHead, "", url.Values{"restype": {"container"}}, nil, nil)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

// do sends a request for key, signed with the account's key.
func (a *Azure) do(ctx context.Context, method, key string, query url.Values, headers map[string]string, body io.Reader) (*http.Response, error) {
	u := *a.base
	key = CleanKey(key)
	decoded := "/" + a.cfg.Container + "/" + key
	encoded := "/" + uriEncode(a.cfg.Container, false) + "/" + uriEncode(key, true)
	u.Path, u.RawPath = decoded, encoded
	if query != nil {
		u.RawQuery = query.Encode()
	}
	req, err := http.NewRequest(method, u.String(), body)
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
	req.Header.Set("x-ms-date", time.Now().UTC().Format(http.TimeFormat))
	req.Header.Set("x-ms-version", "2021-08-06")
	req.Header.Set("Authorization", a.sign(req))
	resp, err := a.client.Do(req)
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

// sign is the SharedKey signature of a request.
func (a *Azure) sign(req *http.Request) string {
	contentLength := ""
	if req.ContentLength > 0 {
		contentLength = fmt.Sprint(req.ContentLength)
	}
	var ms []string
	for k, v := range req.Header {
		if strings.HasPrefix(strings.ToLower(k), "x-ms-") {
			ms = append(ms, strings.ToLower(k)+":"+strings.Join(v, ","))
		}
	}
	sort.Strings(ms)
	canonicalHeaders := strings.Join(ms, "\n") + "\n"
	resource := "/" + a.cfg.Account + req.URL.EscapedPath()
	if q := req.URL.Query(); len(q) > 0 {
		keys := make([]string, 0, len(q))
		for k := range q {
			keys = append(keys, strings.ToLower(k))
		}
		sort.Strings(keys)
		for _, k := range keys {
			resource += "\n" + k + ":" + strings.Join(q[k], ",")
		}
	}
	toSign := strings.Join([]string{
		req.Method, "", "", contentLength, "", req.Header.Get("Content-Type"), "", "", "", "", "", "",
	}, "\n") + "\n" + canonicalHeaders + resource
	sum := hmacSum(a.decodeKey(), toSign)
	return "SharedKey " + a.cfg.Account + ":" + base64.StdEncoding.EncodeToString(sum)
}

// decodeKey is the account's key, base64 as Azure shows it.
func (a *Azure) decodeKey() []byte {
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(a.cfg.AccessKey))
	if err != nil {
		return []byte(a.cfg.AccessKey)
	}
	return key
}
