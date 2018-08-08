package main
import (
    "encoding/json"
    "errors"
    "flag"
    "fmt"
    "html/template"
    "math/rand"
    "net"
    "net/http"
    neturl "net/url"
    "os"
    "strconv"
    "strings"
    "time"

    log "github.com/sirupsen/logrus"
    "github.com/rifflock/lfshook"
    "github.com/jbowtie/gokogiri"

    "github.com/patrickyeon/rssrerun"
)

var templateSources = []string{"about.html", "build.html", "preview.html"}
var templates  = make(map[string]*template.Template)
var weekdays = []time.Weekday{time.Sunday, time.Monday, time.Tuesday,
                              time.Wednesday, time.Thursday, time.Friday,
                              time.Saturday}
var dayNames = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}
var store = rssrerun.NewJSONStore("data/stores/podcasts/")

var CautionNoFetcher = `No auto-builder known.
The server did not auto-detect a method to build up the entire history of the
 feed, and has fallen back to using just the current feed available. This could
 still work, but it's quite possible that it is missing some of the earlier
 items.`
var CautionSketchyFetcher = `Best-guess auto-builder.
The server has made an attempt to re-build the entire history of the feed, but
 the method used is known to sometimes have problems. Most likely, if there's an
 issue, it will be with the earlier items.`
var CautionQualityIssue = `Potential feed quality issues.
A user has flagged a quality issue with this feed. Proceed with caution.`

const (
    gradeFailed = "failed"
    // FIXME actually check for gradeBuilding so that you don't double up
    gradeBuilding = "building"
    gradeAdminBad = "admin-bad"
    gradeUserVbad = "user-vbad"
    gradeUserBad = "user-bad"
    gradeUserGood = "user-good"
    gradeUserPerfect = "user-perfect"
    gradeAutoSuspect = "auto-suspect"
    gradeAutoTrusted = "auto-trusted"
    gradeAdminGood = "admin-good"
)

var LogFile string
var LogVerbose bool
var LogQuiet bool
var BlackholeEnabled bool
var WatchDelay int

func templateWatcher() {
    timestamps := make(map[string]time.Time)
    for _, fn := range templateSources {
        info, _ := os.Stat("public/" + fn)
        timestamps[fn] = info.ModTime()
    }

    for true {
        time.Sleep(time.Duration(WatchDelay) * time.Second)
        for _, fn := range templateSources {
            info, err := os.Stat("public/" + fn)
            if err != nil {
                continue
            }
            if info.ModTime().After(timestamps[fn]) {
                attempt, err := template.ParseFiles("public/" + fn)
                if err != nil {
                    fmt.Printf("Error parsing %s: %s\n", fn, err)
                } else {
                    templates[fn] = attempt
                    fmt.Printf("Reloaded template: %s\n", fn)
                }
                timestamps[fn] = info.ModTime()
            }
        }
    }
}

func templateOrErr(w http.ResponseWriter, name string, data interface{}) httpError {
    var retval httpError
    err := templates[name].Execute(w, data)
    if err != nil {
        retval = httpErr(http.StatusInternalServerError, err)
        errHandler(w, retval)
        fmt.Print(err.Error())
    }
    // feel free to ignore this
    return retval
}

func jsonOrErr(w http.ResponseWriter, status int, dat interface{}) httpError {
    var retval httpError
    msg, err := json.Marshal(dat)
    if err != nil {
        retval = httpErr(http.StatusInternalServerError, err)
        errHandler(w, retval)
    } else {
        w.WriteHeader(status)
        w.Write(msg)
    }
    return retval
}

type httpError interface {
    Status() int
    Error() string
}

type _httpError struct {
    status int
    err error
}

func httpMsg(status int, msg string) httpError {
    return &_httpError{status, errors.New(msg)}
}
func httpErr(status int, err error) httpError {
    return &_httpError{status, err}
}

func (e *_httpError) Error() string {
    return e.err.Error()
}
func (e *_httpError) Status() int {
    return e.status
}

var _blackhole = make(map[string]time.Time)

func isBlackholed(addr string) bool {
    host, _, _ := net.SplitHostPort(addr)
    if expires, banned := _blackhole[host]; banned {
        if expires.Before(time.Now()) {
            delete(_blackhole, host)
            return false
        }
        return true
    }
    return false
}

var _blackholeTracker = make(map[string]([]time.Time))
func blackholeKick(addr string) {
    const maxHits = 10
    const timeWindow = (15 * time.Second)
    const coolDown = time.Hour
    host, _, _ := net.SplitHostPort(addr)
    record := append(_blackholeTracker[host], time.Now())
    if len(record) >= maxHits {
        for i := len(record) - 2; i >= 0; i-- {
            if time.Since(record[i]) > timeWindow {
                record = record[i + 1:]
                break
            }
        }
        if len(record) >= maxHits && time.Since(record[0]) <= timeWindow {
            _blackhole[host] = time.Now().Add(coolDown)
            log.WithFields(log.Fields{"address": host}).Info("blackholed host")
        }
    }
    _blackholeTracker[host] = record
}

func blackhole(w http.ResponseWriter) {
    w.WriteHeader(http.StatusForbidden)
    fmt.Fprint(w, "You've been flagged for causing too many errors too quickly. ")
    fmt.Fprint(w, "Please wait an hour before trying again")
}

type handlerFunc func(http.ResponseWriter, *http.Request)
type handlerFuncErr func(http.ResponseWriter, *http.Request)httpError

func createHandler(name string, fn handlerFuncErr) handlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        tstart := time.Now()
        if BlackholeEnabled && isBlackholed(r.RemoteAddr) {
            blackhole(w)
            return
        }
        id := rand.Int()
        log.WithFields(log.Fields{
            "handlerFunc": name,
            "request": r.URL.String(),
            "from": r.RemoteAddr,
            "id": id,
        }).Info("Received request")

        err := fn(w, r)

        if err != nil {
            log.WithFields(log.Fields{
                "id": id,
                "elapsed": time.Since(tstart).Seconds(),
                "error": err.Error(),
                "status": err.Status(),
            }).Warn("Request failed")
            if BlackholeEnabled {
                blackholeKick(r.RemoteAddr)
            }
        } else {
            log.WithFields(log.Fields{
                "id": id,
                "elapsed": time.Since(tstart).Seconds(),
            }).Info("Request completed")
        }
    }
}

func homeHandler(w http.ResponseWriter, r *http.Request) httpError {
    return templateOrErr(w, "about.html", nil)
}

func previewHandler(w http.ResponseWriter, r *http.Request) httpError {
    req := r.URL.Query()
    sched := []time.Weekday{}
    intsched := ""
    txtsched := []string{}
    for i, d := range dayNames {
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
        return errHandler(w, httpMsg(http.StatusBadRequest,
                                     "Need at least one day in your schedule."))
    }

    if req["url"] == nil {
        return errHandler(w, httpMsg(http.StatusNotFound,
                                     "We don't have that feed yet. Try another?"))
    }
    url := req["url"][0]
    if !store.Contains(url) {
        return errHandler(w, httpMsg(http.StatusNotFound,
                                     "We don't have that feed yet. Try another?"))
    }
    items, err := store.Get(url, 0, nItems)
    if err != nil {
        return errHandler(w, httpErr(http.StatusInternalServerError, err))
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
        ret[nItems - i - 1] = lnk{it.Render().Title, guid,
                                  date.Format("Mon Jan 2 2006"),
                                  oldDates[i].Format("Mon Jan 2 2006")}
    }

    type prevDat struct {
        Title, Url, Weekdays, FeedLink, Warning string
        Items []lnk
    }
    warning := ""
    grade, err := store.GetInfo(url, "grade")
    if err != nil {
        log.WithFields(log.Fields{
            "url": url,
        }).Error("URL has no grade in store")
        warning = CautionQualityIssue
    } else if grade == gradeAutoSuspect {
        warning = CautionSketchyFetcher
    } else if (grade == gradeUserVbad || grade == gradeUserBad ||
               grade == gradeUserGood || grade == gradeAdminBad) {
        warning = CautionQualityIssue
    }
    link := ("/api/feed?url=" + neturl.PathEscape(url) +
             "&start=" + startdate.Format("20060102"))
    link += "&sched=" + intsched
    dat := prevDat{"Your Podcast", url, strings.Join(txtsched, "/"), link,
                   warning, ret}
    return templateOrErr(w, "preview.html", dat)
}

func buildHandler(w http.ResponseWriter, r *http.Request) httpError {
    type buildDat struct {
        ApiStub, Url string
    }
    req := r.URL.Query()
    if req["url"] == nil {
        return errHandler(w, httpMsg(http.StatusBadRequest,
                                     "need a URL to try to build a feed"))
    }
    url := req["url"][0]
    if store.Contains(url) {
        // 302 them
        target := "/preview?url=" + neturl.PathEscape(url)
        for _, day := range dayNames {
            if req[day] != nil {
                target += "&" + day + "="
            }
        }

        w.Header().Add("Location", target)
        w.WriteHeader(http.StatusFound)
        return nil
    }
    dat := buildDat{"/api/build?url=", url}
    return templateOrErr(w, "build.html", dat)
}

func buildApiHandler(w http.ResponseWriter, r *http.Request) httpError {
    req := r.URL.Query()
    if req["url"] == nil {
        return jsonOrErr(w, http.StatusBadRequest, map[string]string{
            "err": "badurl",
            "msg": "no URL provided to build a feed",
        })
    }
    url := req["url"][0]
    _, err := store.CreateIndex(url)
    if err != nil {
        // tell them it already exists, encourage them to sign up
        return jsonOrErr(w, http.StatusBadRequest,
                         map[string]string{"err": "feedexists"})
    }
    err = store.SetInfo(url, "grade", gradeBuilding)
    if err != nil {
        return errHandler(w, httpMsg(http.StatusInternalServerError,
                                     "TODO: error setting grade=building?"))
    }
    caution := ""
    fn, err := rssrerun.SelectFeedFetcher(url)
    gradename := gradeAutoTrusted
    if err == rssrerun.FetcherDetectFailed {
        fn = rssrerun.FeedFromUrl
        caution = CautionNoFetcher
        gradename = gradeAutoSuspect
    } else if err == rssrerun.FetcherDetectUntrusted {
        caution = CautionSketchyFetcher
        gradename = gradeAutoSuspect
    } else if err != nil {
        _ = store.SetInfo(url, "grade", gradeFailed)
        return jsonOrErr(w, http.StatusInternalServerError,
                         map[string]string{
            "err": "rerunerr",
            "msg": err.Error(),
        })
    }
    feed, err := fn(url)
    if err != nil {
        _ = store.SetInfo(url, "grade", gradeFailed)
        return jsonOrErr(w, http.StatusInternalServerError,
                         map[string]string{
            "err": "rerunerr",
            "msg": err.Error(),
        })
    }
    nItems := feed.LenItems()
    revFeed := make([]rssrerun.Item, nItems)
    for i := 0; i < nItems; i++ {
        revFeed[i] = feed.Item(nItems - i - 1)
    }
    store.Update(url, revFeed)
    if nItems < 2 {
        _ = store.SetInfo(url, "grade", gradeAutoSuspect)
        return jsonOrErr(w, http.StatusInternalServerError,
                         map[string]string{
            "err": "rerunerr",
            "msg": "that feed, as rebuilt, looks broken.",
        })
    }
    _ = store.SetInfo(url, "grade", gradename)
    first := renderToMap(feed.Item(nItems - 1).Render())
    last := renderToMap(feed.Item(0).Render())
    return jsonOrErr(w, http.StatusOK, map[string]interface{}{
        "nItems": nItems,
        "first": first,
        "last": last,
        "url": url,
        "caution": caution,
        "askgrade": (gradename != gradeAutoTrusted),
    })
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

func feedApiHandler(w http.ResponseWriter, r *http.Request) httpError {
    req := r.URL.Query()
    if req["url"] == nil || req["start"] == nil || req["sched"] == nil {
        return errHandler(w, httpMsg(http.StatusBadRequest,
                                     "not enough params (need url, start, and sched)"))
    }
    url := req["url"][0]
    if !store.Contains(url) {
        return errHandler(w, httpMsg(http.StatusNotFound,
                                     url + " is not in the store"))
    }

    start, err := time.Parse("20060102", req["start"][0])
    if err != nil {
        return errHandler(w, httpMsg(http.StatusBadRequest,
                                     "invalid date passed as start"))
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
        return errHandler(w, httpErr(http.StatusInternalServerError, err))
    }

    // mangle the pubdates
    for _, it := range(items) {
        nd, _ := ds.NextDate()
        it.SetPubDate(nd)
    }

    // build and return the feed
    w.Header().Add("Content-Type", "text/xml")
    wrapstr, err := store.GetInfo(url, "wrapper")
    if err != nil {
        return errHandler(w, httpErr(http.StatusInternalServerError, err))
    }
    wrap := []byte(wrapstr)
    //  some feeds have an <itunes:new-feed-url> tag to act as a redirect. We
    // are going to strip that out if it exists because we don't want to get
    // overruled by a redirect.
    doc, err := gokogiri.ParseXml(wrap)
    if err != nil {
        return errHandler(w, httpErr(http.StatusInternalServerError, err))
    }
    xp := doc.DocXPathCtx()
    for _, ns := range doc.Root().DeclaredNamespaces() {
        xp.RegisterNamespace(ns.Prefix, ns.Uri)
    }
    searches := []string{}
    for _, ns := range doc.Root().DeclaredNamespaces() {
        if ns.Uri == "http://www.itunes.com/dtds/podcast-1.0.dtd" && len(ns.Prefix) > 0 {
            searches = append(searches, "channel/" + ns.Prefix + ":new-feed-url")
            searches = append(searches, "feed/" + ns.Prefix + ":new-feed-url")
        }
    }
    searches = append(searches, "channel/new-feed-url")
    searches = append(searches, "feed/new-feed-url")
    for _, search := range searches {
        tags, err := doc.Root().Search(search)
        if err != nil {
            continue
        }
        for _, tag := range tags {
            tag.Unlink()
        }
    }
    wrap = doc.ToBuffer(nil)

    fd, _ := rssrerun.NewFeed(wrap, nil)
    // flip the ordering of `items`
    for i := 0; i < len(items) / 2; i++ {
        j := len(items) - i - 1
        items[i], items[j] = items[j], items[i]
    }
    fmt.Fprint(w, string(fd.BytesWithItems(items)))
    return nil
}

func gradeApiHandler(w http.ResponseWriter, r *http.Request) httpError {
    req := r.URL.Query()
    if req["url"] == nil || req["grade"] == nil {
        return errHandler(w, httpMsg(http.StatusBadRequest,
                                     "not enough params (need url, grade)"))
    }
    url := req["url"][0]
    prev, err := store.GetInfo(url, "grade")
    if err != nil {
        return errHandler(w, httpErr(http.StatusInternalServerError, err))
    }
    valid := false
    userGrades := []string{gradeUserVbad, gradeUserBad,
                           gradeUserGood, gradeUserPerfect}
    for _, g := range append(userGrades, gradeAutoSuspect) {
        if prev == g {
            valid = true
            break
        }
    }
    if !valid {
        return errHandler(w, httpMsg(http.StatusUnauthorized,
                                     "trying to override non-user grade"))
    }
    grade := req["grade"][0]
    for _, g := range userGrades {
        if grade == g {
            store.SetInfo(url, "grade", grade)
            return jsonOrErr(w, 200, map[string]string{"status": "ok"})
        }
    }
    return errHandler(w, httpMsg(http.StatusBadRequest,
                                 "trying to set a non-user or invalid grade"))
}

func errHandler(w http.ResponseWriter, err httpError) httpError {
    w.WriteHeader(err.Status())
    fmt.Fprintf(w, "<html><head><title>broken</title></head>")
    fmt.Fprintf(w, "<body>%s</body></html>", err.Error())
    return err
}

func init() {
    for _, fn := range templateSources {
        templates[fn] = template.Must(template.ParseFiles("public/" + fn))
    }

    flag.BoolVar(&LogVerbose, "v", false, "Report info, warn, errors")
    flag.BoolVar(&LogQuiet, "q", false, "Only report errors")
    flag.StringVar(&LogFile, "logfile", "", "File to append logs into")
    flag.BoolVar(&BlackholeEnabled, "blackhole", false, "fail2ban-like protection")
    flag.IntVar(&WatchDelay, "watch", 0, "check for template changes")
}

func main() {
    flag.Parse()

    // set up the logging
    log.SetLevel(log.WarnLevel)
    if LogQuiet {
        log.SetLevel(log.ErrorLevel)
    }
    if LogVerbose {
        log.SetLevel(log.InfoLevel)
    }

    if LogFile != "" {
        logfd, err := os.OpenFile(LogFile,
                                  os.O_WRONLY|os.O_APPEND|os.O_CREATE,
                                  0666)
        if err != nil {
            log.WithFields(log.Fields{
                "filename": LogFile,
            }).Fatal("Could not open/create logfile!")
        }
        defer logfd.Close()
        log.AddHook(lfshook.NewHook(logfd, &log.JSONFormatter{}))
    }

    if WatchDelay > 0 {
        go templateWatcher()
    }

    http.HandleFunc("/", createHandler("home", homeHandler))
    http.HandleFunc("/preview", createHandler("preview", previewHandler))
    http.HandleFunc("/build", createHandler("build", buildHandler))
    http.HandleFunc("/api/feed", createHandler("feedApi", feedApiHandler))
    http.HandleFunc("/api/build", createHandler("buildApi", buildApiHandler))
    http.HandleFunc("/api/grade", createHandler("gradeApi", gradeApiHandler))
    http.Handle("/static/", http.FileServer(http.Dir("public")))
    http.ListenAndServe(":8007", nil)
}
