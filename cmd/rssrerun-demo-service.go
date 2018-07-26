package main
import (
    "encoding/json"
    "fmt"
    "html/template"
    "net/http"
    neturl "net/url"
    "strconv"
    "strings"
    "time"

    "github.com/patrickyeon/rssrerun"
)

var templates = template.Must(template.ParseFiles("public/about.html",
                                                  "public/build.html",
                                                  "public/preview.html"))
var weekdays = []time.Weekday{time.Sunday, time.Monday, time.Tuesday,
                              time.Wednesday, time.Thursday, time.Friday,
                              time.Saturday}
var store = rssrerun.NewJSONStore("data/stores/podcasts/")

var CautionNoFetcher = "No auto-builder known. Just using live feed."

type feedGrade int
const (
    failed feedGrade = iota
    building
    adminBad
    userVbad
    userBad
    userGood
    userPerfect
    autoSuspect
    autoTrusted
    adminGood
)
var gradeNames = []string{"failed", "building", "admin-bad", "user-vbad",
                          "user-bad", "user-good", "user-perfect",
                          "auto-suspect", "auto-trusted", "admin-good"}

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
    } else if len(titletxt) == 0 {
        titletxt = "(no title found)"
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
        errHandler(w, "We don't have that feed yet. Try another?")
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
    link := ("/feed?url=" + neturl.PathEscape(url) +
             "&start=" + startdate.Format("20060102"))
    link += "&sched=" + intsched
    dat := prevDat{"Your Podcast", url, strings.Join(txtsched, "/"), link, ret}
    templateOrErr(w, "preview.html", dat)
}

func buildHandler(w http.ResponseWriter, r *http.Request) {
    type buildDat struct {
        ApiStub, Url string
    }
    req := r.URL.Query()
    if req["url"] == nil {
        errHandler(w, "need a URL to try to build a feed")
        return
    }
    url := req["url"][0]
    // TODO really should have an existence test for url
    nItems := store.NumItems(url)
    if nItems != 0 {
        // tell them it already exists, encourage them to sign up
        errHandler(w, "TODO: feed exists. give it to user")
        return
    }
    dat := buildDat{"/fetch?url=", url}
    templateOrErr(w, "build.html", dat)
}

func fetchApiHandler(w http.ResponseWriter, r *http.Request) {
    req := r.URL.Query()
    if req["url"] == nil {
        errJsonHandler(w, map[string]string{
            "err": "badurl",
            "msg": "no URL provided to build a feed",
        })
        return
    }
    url := req["url"][0]
    _, err := store.CreateIndex(url)
    if err != nil {
        // tell them it already exists, encourage them to sign up
        errJsonHandler(w, map[string]string{"err": "feedexists"})
        return
    }
    err = store.SetInfo(url, "grade", gradeNames[building])
    if err != nil {
        errHandler(w, "TODO: error setting grade=building?")
        return
    }
    caution := ""
    fn, err := rssrerun.SelectFeedFetcher(url)
    gradename := gradeNames[autoTrusted]
    if err == rssrerun.FetcherDetectFailed {
        fn = rssrerun.FeedFromUrl
        caution = CautionNoFetcher
        gradename = gradeNames[autoSuspect]
    } else if err != nil {
        errJsonHandler(w, map[string]string{
            "err": "rerunerr",
            "msg": err.Error(),
        })
        _ = store.SetInfo(url, "grade", gradeNames[failed])
        return
    }
    feed, err := fn(url)
    if err != nil {
        errJsonHandler(w, map[string]string{
            "err": "rerunerr",
            "msg": err.Error(),
        })
        _ = store.SetInfo(url, "grade", gradeNames[failed])
        return
    }
    nItems := feed.LenItems()
    revFeed := make([]rssrerun.Item, nItems)
    for i := 0; i < nItems; i++ {
        revFeed[i] = feed.Item(nItems - i - 1)
    }
    store.Update(url, revFeed)
    if nItems < 2 {
        errJsonHandler(w, map[string]string{
            "err": "rerunerr",
            "msg": "that feed, as rebuilt, looks broken.",
        })
        _ = store.SetInfo(url, "grade", gradeNames[autoSuspect])
        return
    }
    _ = store.SetInfo(url, "grade", gradename)
    first := renderToMap(feed.Item(nItems - 1).Render())
    last := renderToMap(feed.Item(0).Render())
    err = json.NewEncoder(w).Encode(map[string]interface{}{
        "nItems": nItems,
        "first": first,
        "last": last,
        "url": url,
        "caution": caution,
    })
    if err != nil {
        errHandler(w, err.Error())
    }
}

func renderToMap(item rssrerun.RenderItem) map[string]string {
    return map[string]string {
        "pubdate": item.PubDate,
        "title": item.Title,
        "description": item.Description,
        "guid": item.Guid,
        "url": item.Url,
        "enclosure": item.Enclosure,
    }
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

func gradeApiHandler(w http.ResponseWriter, r *http.Request) {
    req := r.URL.Query()
    if req["url"] == nil || req["grade"] == nil {
        errHandler(w, "not enough params (need url, grade)")
        return
    }
    url := req["url"][0]
    prev, err := store.GetInfo(url, "grade")
    if err != nil {
        errHandler(w, err.Error())
        return
    }
    valid := false
    // TODO all of this grading really needs to be organized properly
    for _, g := range gradeNames[userVbad:autoSuspect + 1] {
        if prev == g {
            valid = true
            break
        }
    }
    if !valid {
        errHandler(w, "trying to override non-user grade")
        return
    }
    grade := req["grade"][0]
    switch(grade) {
    case gradeNames[userVbad]:
    case gradeNames[userBad]:
    case gradeNames[userGood]:
    case gradeNames[userPerfect]:
        store.SetInfo(url, "grade", grade)
        break
    default:
        errHandler(w, "trying to set a non-user or invalid grade")
        return
    }

    fmt.Fprintf(w, "ok")
}

func errHandler(w http.ResponseWriter, msg string) {
    fmt.Fprintf(w, "<html><head><title>broken</title></head>")
    fmt.Fprintf(w, "<body>%s</body></html>", msg)
}

func errJsonHandler(w http.ResponseWriter, dat map[string]string) {
    err := json.NewEncoder(w).Encode(dat)
    if err != nil {
        errHandler(w, err.Error())
    }
}

func main() {
    http.HandleFunc("/", homeHandler)
    http.HandleFunc("/preview", previewHandler)
    http.HandleFunc("/build", buildHandler)
    http.HandleFunc("/feed", feedHandler)
    http.HandleFunc("/fetch", fetchApiHandler)
    http.HandleFunc("/grade", gradeApiHandler)
    http.Handle("/static/", http.FileServer(http.Dir("public")))
    http.ListenAndServe(":8007", nil)
}
