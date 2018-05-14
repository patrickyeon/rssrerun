package rssrerun

import (
    "errors"
    "regexp"
)

var linkSplitter = regexp.MustCompile("<([^>]*)>((.|\\n)*)")
var paramSplitter = regexp.MustCompile(";")
var keyvalSplitter = regexp.MustCompile("(\\w+)=\"([^\"]*)\"")

type Memento struct {
    Url string
    Params map[string]string
}

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
            paramMap[kv[1]] = kv[2]
        }
    }
    return Memento{res[1], paramMap}, nil
}
