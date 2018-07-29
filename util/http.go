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
)

var client = &http.Client{}
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

func Get(url string) (*http.Response, error) {
    u, err := neturl.Parse(url)
    if err != nil {
        return nil, err
    }
    if BeSafe {
        bHosts := bannedHosts()
        for i := 0; i < len(bHosts); i++ {
            // TODO this needs to be checked on redirects as well
            if u.Hostname() == bHosts[i] {
                return nil, ErrorBannedHost
            }
        }
        if ip := net.ParseIP(u.Hostname()); ip != nil && ip.IsLoopback() {
            return nil, ErrorBannedHost
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
