package rssrerun

import (
    "fmt"
    "io"
    "net/http"
    "net/http/httptest"
    "os"
    "strings"
    "testing"
    "time"
)

type testMemento struct {
    Url string
    Params map[string]string
    Unparsed string
}

var testValues = []testMemento{{
    "http://example.com/foo/bar",
    map[string]string{"rel": "original",
                      "datetime": "Fri, 02 Jun 2017 21:27:18 GMT"},
    "<http://example.com/foo/bar>; rel=\"original\"; datetime=\"Fri, 02 Jun 2017 21:27:18 GMT\",",
    },
    {"http://example.com/bar/bar",
    map[string]string{"rel": "original",
                      "foo": "bar"},
    "<http://example.com/bar/bar>;\n rel=\"original\";       foo=\"bar\"",
    },
}

func TestParseLinks (t *testing.T) {
    for _, tm := range testValues {
        link, err := ParseMemento(tm.Unparsed)
        if err != nil {
            t.Fatal(err)
        }
        if link.Url != tm.Url {
            t.Fatalf("link parsing failed. Expected: '%s' Got: '%s'",
                     tm.Url, link.Url)
        }
        for k, v := range tm.Params {
            if link.Params[k] != v {
                t.Fatalf("param parsing failed for %s. Expected: '%s' Got: '%s'",
                         k, v, link.Params[k])
            }
        }
    }
}

func TestExtractLinks (t *testing.T) {
    reader, err := os.Open("testdat/timemap.txt")
    if err != nil {
        t.Fatal(err)
    }
    tm, err := ParseTimeMap(reader)
    if err != nil {
        t.Fatal(err)
    }
    if nlinks := len(tm.Links); nlinks != 7 {
        t.Fatalf("Didn't parse all links. Expected 7, got %d.", nlinks)
    }
}

func TestGetMementos (t *testing.T) {
    reader, err := os.Open("testdat/timemap.txt")
    if err != nil {
        t.Fatal(err)
    }
    tm, err := ParseTimeMap(reader)
    if err != nil {
        t.Fatal(err)
    }
    if nmem := len(tm.GetMementos()); nmem != 4 {
        t.Fatalf("Didn't get all mementos. Expected 4, got %d.", nmem)
    }
}

func TestTMap(t *testing.T) {
    tm := initTMap("http://example.com", "http://timegate.com/example.com")
    tm.addMemento("http://timegate.com/2/example.com", mkDate(2018, 4, 4))
    tm.addMemento("http://timegate.com/1/example.com", mkDate(2018, 2, 3))
    res, err := ParseTimeMap(tm.toReader())
    if err != nil {
        t.Fatal(err)
    }
    if nlinks := len(res.Links); nlinks != 4 {
        t.Errorf("Didn't get all links. Expected 4, got %d.", nlinks)
    }
    if nmem := len(res.GetMementos()); nmem != 2 {
        t.Fatalf("Didn't get all mementos. Expected 2, got %d.", nmem)
    }
}

func TestSeriesOfTimeMaps(t *testing.T) {
    mementos := []string{
        "http://timegate.com/1/example.com",
        "http://timegate.com/2/example.com",
        "http://timegate.com/3/example.com",
        "http://timegate.com/4/example.com",
        "http://timegate.com/5/example.com",
    }

    tm1 := initTMap("http://example.com", "http://timegate.com/example.com")
    for i := 0; i < 2; i++ {
        tm1.addMemento(mementos[i], mkDate(2018, 3, 30 - i))
    }
    ts1 := tmServer(tm1)
    defer ts1.Close()
    tm2 := initTMap("http://example.com", "http://timegate.com/b/example.com")
    for i := 2; i < 5; i++ {
        tm2.addMemento(mementos[i], mkDate(2018, 3, 30 - i))
    }
    tm2.addTMap(ts1.URL)
    ts2 := tmServer(tm2)
    defer ts2.Close()

    timemap, err := SpiderTimeMap(ts2.URL)
    if err != nil {
        t.Fatal(err)
    }
    // should have 7 links, "original", "timegate", then 5 mementos
    // the other "timegate" and the "timemap"s get swallowed silently
    if nlinks := len(timemap.Links); nlinks != 7 {
        t.Errorf("Didn't get all links. Expected 7, got %d.", nlinks)
    }
    result := timemap.GetMementos()
    if nmems := len(result); nmems != 5 {
        t.Errorf("Didn't get all mementos. Expected 5, got %d.", nmems)
    }
    for i := 0; i < len(mementos); i++ {
        if result[i].Url != mementos[i] {
            t.Errorf("Out of order, expected %s got %s.",
                     mementos[i], result[i].Url)
        }
    }
}

func TestCycleOfTimeMaps(t *testing.T) {
    mementos := []string{
        "http://timegate.com/1/example.com",
        "http://timegate.com/2/example.com",
        "http://timegate.com/3/example.com",
        "http://timegate.com/4/example.com",
        "http://timegate.com/5/example.com",
    }
    tm1 := initTMap("http://example.com", "http://timegate.com/example.com")
    ts1 := tmServer(tm1)
    defer ts1.Close()
    tm2 := initTMap("http://example.com", "http://timegate.com/b/example.com")
    tm2.addTMap(ts1.URL)
    ts2 := tmServer(tm2)
    defer ts2.Close()
    for i := 0; i < 2; i++ {
        tm1.addMemento(mementos[i], mkDate(2008, 4, 30 - i))
    }
    for i := 2; i < 5; i++ {
        tm2.addMemento(mementos[i], mkDate(2008, 4, 30 - i))
    }
    tm1.addTMap(ts2.URL)

    //  this will also prove out that we can modify an underlying tmap after
    // calling `tmServer`
    timemap, err := SpiderTimeMap(ts1.URL)
    if err != nil {
        t.Fatal(err)
    }
    mems := timemap.GetMementos()
    if len(mems) != 5 {
        t.Errorf("Didn't get all mementos. Expected 5, got %d.", len(mems))
    }
    for i := 0; i < len(mementos); i++ {
        if mems[i].Url != mementos[i] {
            t.Errorf("Out of order, expected %s got %s.",
                     mementos[i], mems[i].Url)
        }
    }
}

func TestOverlapTimeMaps(t *testing.T) {
    mementos := []string{
        "http://timegate.com/1/example.com",
        "http://timegate.com/2/example.com",
        "http://timegate.com/3/example.com",
        "http://timegate.com/4/example.com",
        "http://timegate.com/5/example.com",
    }
    tm1 := initTMap("http://example.com", "http://timegate.com/example.com")
    ts1 := tmServer(tm1)
    defer ts1.Close()
    tm2 := initTMap("http://example.com", "http://timegate.com/b/example.com")
    tm2.addTMap(ts1.URL)
    ts2 := tmServer(tm2)
    defer ts2.Close()
    // note that mementos[2] is added twice
    for i := 0; i < 3; i++ {
        tm1.addMemento(mementos[i], mkDate(2008, 4, 30 - i))
    }
    for i := 2; i < 5; i++ {
        tm2.addMemento(mementos[i], mkDate(2008, 4, 30 - i))
    }

    timemap, err := SpiderTimeMap(ts2.URL)
    if err != nil {
        t.Fatal(err)
    }
    mems := timemap.GetMementos()
    if len(mems) != 5 {
        t.Errorf("Didn't get all mementos. Expected 5, got %d.", len(mems))
    }
    for i := 0; i < len(mementos); i++ {
        if mems[i].Url != mementos[i] {
            t.Errorf("Out of order, expected %s got %s.",
                     mementos[i], mems[i].Url)
        }
    }
}


// tools used to build timemaps, test the fetching, etc.
type mmnto struct {
    url string
    dt time.Time
}

type tMap struct {
    original string
    timegate string
    timemaps []string
    mementos []mmnto
}

func initTMap(original string, timegate string) *tMap {
    return &tMap{original, timegate, nil, nil}
}
func (t *tMap) addMemento(url string, dt time.Time) {
    t.mementos = append(t.mementos, mmnto{url, dt})
}
func (t *tMap) addTMap(url string) {
    t.timemaps = append(t.timemaps, url)
}
func (t *tMap) toReader() io.Reader {
    return strings.NewReader(t.toString())
}
func (t *tMap) toString() string {
    links := ""
    for _, tmap := range(t.timemaps) {
        links = strings.Join([]string{links,
            "<", tmap, ">; rel=\"timemap\",\n"}, "")
    }
    for _, mm := range(t.mementos) {
        links = strings.Join([]string{links,
            "<", mm.url, ">; rel=\"memento\";",
            "datetime=\"", mm.dt.String(), "\",\n"}, "")
    }
    return strings.Join(append([]string{
        "<", t.original, ">; rel=\"original\",\n",
        "<", t.timegate, ">; rel=\"timegate\",\n",
        }, links), "")
}

func tmServer(t *tMap) *httptest.Server {
    return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,
                                                    r *http.Request) {
            fmt.Fprint(w, t.toString())
    }))
}

func mkDate(year, month, day int) time.Time {
    return time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
}
