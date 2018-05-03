package main
import (
    "fmt"
    "html/template"
    "net/http"
    "strconv"
    "strings"
    "time"

    "github.com/patrickyeon/rssrerun"
)

var templates = template.Must(template.ParseFiles("public/about.html",
                                                  "public/preview.html"))
var weekdays = []time.Weekday{time.Sunday, time.Monday, time.Tuesday,
                              time.Wednesday, time.Thursday, time.Friday,
                              time.Saturday}
var store = rssrerun.NewJSONStore("data/stores/podcasts/")

func templateOrErr(w http.ResponseWriter, name string, data interface{}) error {
    err := templates.ExecuteTemplate(w, name, data)
    if err != nil {
        errHandler(w, err.Error())
        fmt.Print(err.Error())
    }
    // feel free to ignore this
    return err
}

func titleish(item rssrerun.Item) string {
    var titletxt string
    title, err := item.Node().Search("title")
    if err != nil || len(title) == 0 {
        title, err = item.Node().Search("description")
        if err != nil || len(title) == 0 {
            // something there should have succeeded, but tough luck
            titletxt = "(no title found)"
        } else {
            titletxt = title[0].Content()
        }
    } else {
        titletxt = title[0].Content()
    }
    if len(titletxt) > 150 {
        titletxt = titletxt[0:147] + "..."
    }
    return titletxt
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
    templateOrErr(w, "about.html", nil)
}

func previewHandler(w http.ResponseWriter, r *http.Request) {
    req := r.URL.Query()
    sched := []time.Weekday{}
    intsched := ""
    txtsched := []string{}
    for i, d := range strings.Split("sun mon tue wed thu fri sat", " ") {
        if _, t := req[d]; t {
            sched = append(sched, weekdays[i])
            intsched += strconv.Itoa(i)
            txtsched = append(txtsched, weekdays[i].String())
        }
    }

    startdate := time.Now().AddDate(0, 0, -7)
    ds := rssrerun.NewDateSource(startdate, sched)
    nItems := ds.DatesInRange(startdate, time.Now())
    if nItems == 0 {
        errHandler(w, "Need at least one day in your schedule.")
        return
    }

    if req["podcast"] == nil {
        errHandler(w, "We don't have that feed yet. Try another?")
        return
    }
    url := req["podcast"][0]
    if store.NumItems(url) <= 0 {
        errHandler(w, url + " is not in the store")
        return
    }
    items, err := store.Get(url, 0, nItems)
    if err != nil {
        errHandler(w, err.Error())
        return
    }
    // fake out the pubdates on those items
    oldDates := make([]time.Time, len(items))
    for i, it := range(items) {
        nd, _ := ds.NextDate()
        oldDates[i], _ = it.PubDate()
        it.SetPubDate(nd)
    }

    type lnk struct {
        Title, Link, NewDate, OldDate string
    }
    ret := make([]lnk, nItems)
    for i, it := range items {
        date, _ := it.PubDate()
        guid, _ := it.Guid()
        ret[nItems - i - 1] = lnk{titleish(it), guid,
                                  date.Format("Mon Jan 2 2006"),
                                  oldDates[i].Format("Mon Jan 2 2006")}
    }

    type prevDat struct {
        Title, Url, Weekdays, FeedLink string
        Items []lnk
    }
    link := "/feed?url=" + url + "&start=" + startdate.Format("20060102")
    link += "&sched=" + intsched
    dat := prevDat{"Your Podcast", url, strings.Join(txtsched, "/"), link, ret}
    templateOrErr(w, "preview.html", dat)
}

func feedHandler(w http.ResponseWriter, r *http.Request) {
    req := r.URL.Query()
    if req["url"] == nil || req["start"] == nil || req["sched"] == nil {
        errHandler(w, "not enough params (need url, start, and sched)")
        return
    }
    url := req["url"][0]
    if store.NumItems(url) <= 0 {
        errHandler(w, url + " is not in the store")
        return
    }

    start, err := time.Parse("20060102", req["start"][0])
    if err != nil {
        errHandler(w, "invalid date passed as start")
        return
    }

    dates := strings.Split(req["sched"][0], "")
    if len(dates) > 7 {
        // whatever, friend
        dates = dates[0:7]
    }
    sched := make([]time.Weekday, len(dates))
    for i, c := range dates {
        if n, err := strconv.Atoi(c); err == nil {
            sched[i] = weekdays[n]
        }
    }

    ds := rssrerun.NewDateSource(start, sched)
    nItems := ds.DatesInRange(start, time.Now())
    if max := store.NumItems(url); nItems > max {
        nItems = max
    }

    // get the actual items
    var items []rssrerun.Item
    if nItems >= 5 {
        items, err = store.Get(url, nItems - 5, nItems)
        ds.SkipForward(nItems - 5)
    } else {
        items, err = store.Get(url, 0, nItems)
    }
    if err != nil {
        errHandler(w, err.Error())
        return
    }

    // mangle the pubdates
    for i, it := range(items) {
        nd, _ := ds.NextDate()
        if i < 0 || i >= len(items) {
            errHandler(w, "invalid i: " + strconv.Itoa(i) +
                          " len(items): " + strconv.Itoa(len(items)))
            return
        }
        it.SetPubDate(nd)
    }

    // build and return the feed
    w.Header().Add("Content-Type", "text/xml")
    wrap, err := store.GetInfo(url, "wrapper")
    if err != nil {
        errHandler(w, err.Error())
        return
    }
    fd, _ := rssrerun.NewFeed([]byte(wrap), nil)
    // flip the ordering of `items`
    for i := 0; i < len(items) / 2; i++ {
        j := len(items) - i - 1
        items[i], items[j] = items[j], items[i]
    }
    fmt.Fprint(w, string(fd.BytesWithItems(items)))
}

func errHandler(w http.ResponseWriter, msg string) {
    fmt.Fprintf(w, "<html><head><title>broken</title></head>")
    fmt.Fprintf(w, "<body>%s</body></html>", msg)
}

func main() {
    http.HandleFunc("/", homeHandler)
    http.HandleFunc("/preview", previewHandler)
    http.HandleFunc("/feed", feedHandler)
    http.ListenAndServe(":8007", nil)
}
