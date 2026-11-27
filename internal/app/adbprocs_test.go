package app

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// ps's columns become a process, with its RSS in bytes.
func TestParseADBProcs(t *testing.T) {
	out := `PID USER         RSS   VSZ S NAME
1   root         1024  2048 S init
123 u0_a123     20480 40960 S com.foo
`
	list, err := parseADBProcs(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("%d processes: %+v", len(list), list)
	}
	if p := list[1]; p.PID != 123 || p.User != "u0_a123" || p.RSS != 20480*1024 || p.State != "S" || p.Name != "com.foo" {
		t.Errorf("the process is %+v", p)
	}
	if _, err := parseADBProcs("ps: unknown option"); err == nil {
		t.Error("an error read as processes")
	}
}

// The list is filtered by what it shows and sorted as a column asks.
func TestADBProcsShown(t *testing.T) {
	st := &adbProcsState{list: []adbProc{
		{PID: 2, User: "root", Name: "system_server", RSS: 300},
		{PID: 1, User: "root", Name: "init", RSS: 100},
		{PID: 3, User: "u0_a1", Name: "com.foo", RSS: 200},
	}, filter: "com"}
	if got := st.shown(); len(got) != 1 || got[0].PID != 3 {
		t.Errorf("filtered %+v", got)
	}
	st.filter = ""
	st.sort = ui.SortOrder{Column: "mem", Descending: true}
	if got := st.shown(); got[0].PID != 2 || got[2].PID != 1 {
		t.Errorf("sorted %+v", got)
	}
}
