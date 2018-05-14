package rssrerun

import (
    "testing"
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

