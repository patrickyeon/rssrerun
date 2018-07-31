package util

import (
    "bytes"
    "errors"
    "fmt"
    "io"
    "io/ioutil"
    "net"
    "net/http"
    neturl "net/url"
    "time"
)

var client = &http.Client{
    nil,
    filterRedirect,
    nil,
    20 * time.Second,
}
var userAgent = "rssrerunFetcher/0.1"

var _bannedHosts = []string{}
func bannedHosts() []string {
    if len(_bannedHosts) == 0 {
        _bannedHosts = []string{"localhost"}
        addrs, _ := net.InterfaceAddrs()
        for _, addr := range addrs {
            _bannedHosts = append(_bannedHosts, addr.String())
            hosts, err := net.LookupAddr(addr.String())
            if err == nil {
                _bannedHosts = append(_bannedHosts, hosts...)
            }
        }
    }
    return _bannedHosts
}

var BeSafe = true

var ErrorBannedHost = errors.New("Trying to fetch from a banned host")
var ErrorTimeout = errors.New("Fetch took too long (>20 seconds).")
var ErrorTooManyRedirects = errors.New("Too many redirects (>10).")

func filterRedirect(req *http.Request, via []*http.Request) error {
    if len(via) >= 10 {
        return ErrorTooManyRedirects
    }
    if BeSafe {
        return urlCheck(req.URL)
    }
    return nil
}

func urlCheck(url *neturl.URL) error {
    bHosts := bannedHosts()
    for i := 0; i < len(bHosts); i++ {
        if url.Hostname() == bHosts[i] {
            return ErrorBannedHost
        }
    }
    if ip := net.ParseIP(url.Hostname()); ip != nil {
        // host is a raw IP, 
        if ip.IsLoopback() {
            return ErrorBannedHost
        }
    } else {
        ips, err := net.LookupIP(url.Hostname())
        if err != nil {
            return err
        }
        for _, ip := range ips {
            if ip.IsLoopback() {
                return ErrorBannedHost
            }
        }
    }
    return nil
}

func Get(url string) (*http.Response, error) {
    u, err := neturl.Parse(url)
    if err != nil {
        return nil, err
    }
    if BeSafe {
        if err = urlCheck(u); err != nil {
            return nil, err
        }
    }

    req, err := http.NewRequest("GET", url, nil)
    if err != nil {
        return nil, err
    }
    req.Header.Set("user-agent", userAgent)

    return client.Do(req)
}

func LimitedBody(url string, maxBytes int) (*http.Response, error) {
    resp, err := Get(url)
    if err != nil {
        return resp, err
    }
    //  yeah, there's kind of an extra lap here, but I want to be able to notify
    // when we truncate.
    data, err := ioutil.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)))
    resp.Body.Close()
    if err == nil && len(data) >= maxBytes {
        err = errors.New(fmt.Sprint("Content truncated at %dB.", maxBytes))
    }
    resp.Body = ioutil.NopCloser(bytes.NewReader(data))
    return resp, err
}

// canonicalize an `url` by following any redirects until we get data
func CanonicalUrl(url string) (string, error) {
    data, err := Get(url)
    data.Body.Close()
    if err != nil {
        return "", err
    }
    if data.StatusCode >= 400 {
        return "", errors.New(data.Status)
    }
    return data.Request.URL.String(), nil
}
