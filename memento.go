package rssrerun

import (
    "bufio"
    "bytes"
    "errors"
    "io"
    "regexp"
    "sort"
    "strings"
    "time"

    "github.com/patrickyeon/rssrerun/util"
)

var linkSplitter = regexp.MustCompile("<([^>]*)>((.|\\n)*)")
var paramSplitter = regexp.MustCompile(";")
var keyvalSplitter = regexp.MustCompile("(\\w+)=\"([^\"]*)\"")

type Memento struct {
    Url string
    Params map[string]string
}

var RFC1123date = "Mon, 02 Jan 2006 15:04:05 GMT"

type memslice []Memento

func nilMemento() (Memento) {
    return Memento{"", nil}
}

func ParseMemento(s string) (Memento, error) {
    res := linkSplitter.FindStringSubmatch(s)
    if res == nil {
        return nilMemento(), errors.New("parse regex didn't match")
    }
    link := res[1]
    params := res[2]
    if link == "" {
        return nilMemento(), errors.New("no link parsed out")
    }
    paramMap := make(map[string]string)
    if params != "" {
        for _, match := range paramSplitter.Split(params, -1) {
            kv := keyvalSplitter.FindStringSubmatch(match)
            if kv == nil {
                continue
            }
            if kv[1] == "datetime" {
                var err error
                kv[2], err = canonDate(kv[2])
                if err != nil {
                    return nilMemento(), err
                }
            }
            paramMap[kv[1]] = kv[2]
        }
    }
    return Memento{res[1], paramMap}, nil
}

func canonDate(s string) (string, error) {
    // try to parse a string date, return it as a proper date string
    //  RFC1123 has a strict interpretation of what this should be, but I don't
    // trust the world at large
    formats := []string{RFC1123date,
                        "Mon, 2 Jan 2006, 15:04:05 GMT"}
    for _, f := range(formats) {
        t, err := time.Parse(f, s)
        if err == nil {
            return t.Format(formats[0]), nil
        }
    }
    return s, errors.New(strings.Join([]string{"Could not parse date ", s,
                                               "It should be of format ",
                                               formats[0]}, ""))
}

type TimeMap struct {
	Links []Memento
}

func ParseTimeMap(r io.Reader) (*TimeMap, error) {
    retval := TimeMap{[]Memento{}}
    scanner := bufio.NewScanner(r)
    var agg bytes.Buffer
    for scanner.Scan() {
        _, _ = agg.WriteString(scanner.Text())
        ss := agg.String()
        matched, _ := regexp.MatchString(".*,\\s*$", ss)
        if matched || scanner.Err() != nil {
            mem, err := ParseMemento(agg.String())
            if err != nil {
                return nil, err
            }
            retval.Links = append(retval.Links, mem)
            agg.Reset()
        }
    }
    return &retval, nil
}

func FetchTimeMap(url string) (*TimeMap, error) {
    res, err := util.Get(url)
    defer res.Body.Close()
    if err != nil {
        return nil, err
    }
    if res.StatusCode <= 400 {
        return nil, errors.New(res.Status)
    }
    return ParseTimeMap(res.Body)
}

func inArray(needle string, haystack []string) bool {
    for _, checkme := range haystack {
        if needle == checkme {
            return true
        }
    }
    return false
}

// be able to use the `sort` library
func (mem memslice) Less(i, j int) bool {
    idate, _ := time.Parse(RFC1123date, mem[i].Params["datetime"])
    jdate, _ := time.Parse(RFC1123date, mem[j].Params["datetime"])
    return idate.After(jdate)
}
func (mem memslice) Swap(i, j int) {
    temp := mem[i]
    mem[i] = mem[j]
    mem[j] = temp
}
func (mem memslice) Len() int {
    return len(mem)
}

//  Recurse through any timemaps linked, also fetching from them. Return with 
// a TimeMap that has all of the mementos, but no hint that there were other
// TimeMaps.
func SpiderTimeMap(url string) (*TimeMap, error) {
    tmap, err := recurseSpider(url, nil)
    if err != nil {
        return nil, err
    }
    mementos := tmap.GetMementos()
    others := []Memento{}
    for i := 0; i < len(tmap.Links); i++ {
        if (tmap.Links[i].Params["rel"] != "timemap" &&
            !strings.HasSuffix(tmap.Links[i].Params["rel"], "memento")) {
            others = append(others, tmap.Links[i])
        }
    }
    sort.Sort(memslice(mementos))
    last := 0
    for i := 1; i < len(mementos); i++ {
        //  need to get rid of any duplicate URLs. Making the assumption that
        // they will have the same datetime, so will have sorted right beside
        // each other.
        if mementos[last].Url != mementos[i].Url {
            last++
            if last != i {
                mementos[last] = mementos[i]
            }
        }
    }

    tmap.Links = append(others, mementos[:last + 1]...)
    return tmap, nil
}

func recurseSpider(url string, skip_urls []string) (*TimeMap, error) {
    skip_urls = append(skip_urls, url)
    tm, err := FetchTimeMap(url)
    if err != nil {
        return nil, err
    }
    for _, link := range tm.GetTimeMaps() {
        if !inArray(link.Url, skip_urls) {
            subtm, err := recurseSpider(link.Url, skip_urls)
            if err != nil {
                return nil, err
            }
            tm.Links = append(tm.Links, subtm.GetTimeMaps()...)
            tm.Links = append(tm.Links, subtm.GetMementos()...)
            for _, tmap := range subtm.GetTimeMaps() {
                skip_urls = append(skip_urls, tmap.Url)
            }
        }
    }
    return tm, nil
}

func (tm *TimeMap) GetTimeMaps() []Memento {
    retval := []Memento{}
    for _, link := range tm.Links {
        if link.Params["rel"] == "timemap" {
            retval = append(retval, link)
        }
    }
    return retval
}

func (tm *TimeMap) GetMementos() []Memento {
    retval := []Memento{}
    for _, link := range tm.Links {
        if strings.HasSuffix(link.Params["rel"], "memento") {
            retval = append(retval, link)
        }
    }
    return retval
}
