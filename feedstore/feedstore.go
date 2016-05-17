package feedstore

import (
    "crypto/md5"
    "encoding/hex"
    "encoding/json"
    "errors"
    "io/ioutil"
    "net/http"
    "os"
    "strconv"

    "github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

/* index json obj:
{'url': 'actual url used',
 'count': 'number of items',
 'hash': 'actual hash',
 'others': {$url: $hash},
 'guids': [set_of_stashed_guids],
 'meta': {$key: $val} // for external use
}
*/

type Index struct {
    Url string `json:"url"`
    Count int `json:"count"`
    Hash string `json:"hash"`
    Guids []string `json:"guids"`
    Others map[string]string `json:"others"`
    Meta map[string]string `json:"meta"`
}

/* store subdirectory:
/index.json (info)
/0.xml (items 0-9)
/1.xml (items 1-19)
 [...]
/n.xml (items n*10 - max)
 items are stored as <item> elements, oldest first, as siblings under one parent
  <xml> tag.
*/

type Store struct {
    rootdir string
    key func(string)string
    canon func(string) (string, error)
}

var canonCache = make(map[string]string)

func NewStore(dir string) *Store {
    // expand dir to canonical rep
    // make sure it exists
    ret := new(Store)
    ret.rootdir = dir
    ret.key = justmd5
    ret.canon = cachingFollowHttp
    return ret
}

func justmd5(url string) string {
    ret := md5.Sum([]byte(url))
    return hex.EncodeToString(ret[:])
}

func followHttp(url string) (string, error) {
    data, err := http.Get(url)
    if err != nil {
        return "", err
    }
    if stat := data.StatusCode; stat != 200 {
        return "", errors.New("HTTP error " + strconv.Itoa(stat))
    }
    return data.Request.URL.String(), nil
}

func cachingFollowHttp(url string) (string, error) {
    if canonCache[url] != "" {
        return canonCache[url], nil
    }
    ret, err := followHttp(url)
    if err == nil {
        canonCache[url] = ret
    }
    return ret, err
}

func fileof(s *Store, ind Index, item int) string {
    retval := s.rootdir + ind.Hash + "/"
    if item == -1 {
        // special case, index
        retval += "index.json"
    } else if item >= 0 {
        retval += strconv.Itoa(item / 10) + ".xml"
    }
    return retval
}

func (s *Store) indexFor(url string) (Index, error) {
    url, err := s.canon(url)
    if err != nil {
        return Index{}, err
    }
    ind, err := s.indexForHash(s.key(url))
    if err != nil {
        return Index{}, err
    }

    if ind.Url != url {
        for key := range ind.Others {
            if key == url {
                return s.indexForHash(ind.Others[key])
            }
        }
        return Index{}, errors.New("couldn't find url")
    }

    return ind, nil
}

func (s *Store) indexForHash(hash string) (Index, error) {
    index, err := os.Open(s.rootdir + hash + "/index.json")
    if err != nil {
        return Index{}, err
    }
    dat, err := ioutil.ReadAll(index)
    if err != nil {
        return Index{}, err
    }
    ind := Index{}
    json.Unmarshal(dat, &ind)
    _ = index.Close()
    return ind, nil
}

func (s *Store) CreateIndex(url string) (Index, error) {
    url, err := s.canon(url)
    if err != nil {
        return Index{}, err
    }
    hash := s.key(url)
    _, err = s.indexFor(url)
    if err == nil {
        return Index{}, errors.New("Index already exists")
    }

    ind := Index{}
    ind.Url = url
    parent, err := s.indexForHash(hash)
    if err == nil {
        // there is a collision
        ind.Hash = parent.Hash + "-" + strconv.Itoa(len(ind.Others))
        if parent.Others == nil {
            parent.Others = make(map[string]string)
        }
        parent.Others[url] = ind.Hash
        s.saveIndex(parent)
    } else {
        ind.Hash = hash
    }
    err = os.Mkdir(s.rootdir + ind.Hash, os.ModeDir | os.ModePerm)
    if err != nil {
        return Index{}, err
    }
    err = s.saveIndex(ind)
    if err != nil {
        return Index{}, err
    }

    return ind, nil
}

func (s *Store) Get(url string, start int, end int) ([]xml.Node, error) {
    // we will return an array of items, oldest first, of length (end - start)
    index, err := s.indexFor(url)
    if err != nil {
        return nil, err
    }
    return s.getInd(index, start, end)
}

func (s *Store) getInd(index Index, start int, end int) ([]xml.Node, error) {
    if start < 0 || end <= start {
        return nil, errors.New("invalid range")
    }
    if end > index.Count {
        return nil, errors.New("invalid range")
    }

    ret := make([]xml.Node, end - start)
    var items []xml.Node

    fname := ""
    for i := start; i < end; i++ {
        if fname != fileof(s, index, i) {
            fname = fileof(s, index, i)
            f, err := os.Open(fname)
            if err != nil {
                return nil, err
            }
            ftxt, err := ioutil.ReadAll(f)
            f.Close()
            if err != nil {
                return nil, err
            }
            // TODO this is all fairly janky, as we build up an xml document,
            //      and then parse it just to get it split into items.
            ftxt = append([]byte("<xml>\n"), ftxt...)
            ftxt = append(ftxt, []byte("</xml>")...)
            itXml, err := gokogiri.ParseXml(ftxt)
            if err != nil {
                return nil, err
            }
            items, err = itXml.Root().Search("//item")
            if err != nil {
                return nil, err
            }
        }
        ret[i - start] = items[i % 10]
    }
    return ret, nil
}

func (s *Store) NumItems(url string) int {
    ind, err := s.indexFor(url)
    if err != nil {
        return 0
    }
    return ind.Count
}

func (s *Store) saveIndex(index Index) error {
    ser, err := json.Marshal(index)
    if err != nil {
        return err
    }

    f, err := os.Create(s.rootdir + index.Hash + "/index.json")
    if err != nil {
        return err
    }

    f.Write(ser)
    f.Close()
    return nil
}

func getGuid(item xml.Node) (string, error) {
    // come on, let's hope for a proper guid
    gtag, err := item.Search("guid")
    if err == nil && len(gtag) > 0 {
        return gtag[0].Content(), nil
    }
    title, err := item.Search("title")
    if err != nil {
        return "", err
    }
    link, err := item.Search("link")
    if err != nil {
        return "", err
    }
    if len(link) == 0 || len(title) == 0 {
        return "", errors.New("can't build a guid")
    }
    return title[0].Content() + " - " + link[0].Content(), nil
}

func (s *Store) Update(url string, items []xml.Node) error {
    // items must be passed in oldest first
    ind, err := s.indexFor(url)
    if err != nil {
        return err
    }
    lastind := ind.Count - 1
    storefile, err := os.OpenFile(fileof(s, ind, lastind),
                                  os.O_APPEND | os.O_WRONLY, os.ModePerm)
    if os.IsNotExist(err) {
        storefile, err = os.Create(fileof(s, ind, lastind))
    }
    if err != nil {
        return err
    }

    // keep track of guids in a set
    guids := make(map[string]bool)
    for _, g := range ind.Guids {
        guids[g] = true
    }

    for _, it := range items {
        guid, err := getGuid(it)
        if err != nil {
            return err
        }
        if _, found := guids[guid]; found {
            continue
        }

        lastind++
        if lastind % 10  == 0 {
            storefile.Close()
            storefile, err = os.Create(fileof(s, ind, lastind))
            if err != nil {
                return err
            }
        }
        _, err = storefile.WriteString(it.String() + "\n")
        if err != nil {
            storefile.Close()
            return err
        }
        guids[guid] = true
    }
    ind.Guids = make([]string, len(guids))
    i := 0
    for g, _ := range guids {
        ind.Guids[i] = g
        i++
    }
    storefile.Close()
    ind.Count = lastind + 1
    err = s.saveIndex(ind)
    if err != nil {
        return err
    }
    return nil
}

func (s *Store) GetInfo(url string, key string) (string, error) {
    ind, err := s.indexFor(url)
    if err != nil {
        return "", err
    }
    return ind.Meta[key], nil
}

func (s *Store) SetInfo(url string, key string, val string) error {
    ind, err := s.indexFor(url)
    if err == nil {
        if ind.Meta == nil {
            ind.Meta = make(map[string]string)
        }
        ind.Meta[key] = val
        err = s.saveIndex(ind)
    }
    return err
}
