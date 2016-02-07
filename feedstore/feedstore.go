package feedstore

import (
    "crypto/md5"
    "encoding/hex"
    "encoding/json"
    "errors"
    "io/ioutil"
    "os"
    "strconv"

    //"github.com/moovweb/gokogiri"
    "github.com/moovweb/gokogiri/xml"
)

/* index json obj:
{'url': 'actual url used',
 'count': 'number of items',
 'hash': 'actual hash',
 //'others': {$url: $hash},
 'guids': [set_of_stashed_guids]
}
*/

type Index struct {
    Url string `json:"url"`
    Count int `json:"count"`
    Hash string `json:"hash"`
    Guids []string `json:"guids"`
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
}

func NewStore(dir string) *Store {
    // expand dir to canonical rep
    // make sure it exists
    ret := new(Store)
    ret.rootdir = dir
    return ret
}

func key(url string) string {
    ret := md5.Sum([]byte(url))
    return hex.EncodeToString(ret[:])
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
    hash := key(url)
    index, err := os.Open(s.rootdir + hash + "/index.json")
    ind := Index{}
    if err != nil {
        if os.IsNotExist(err) {
            err = os.Mkdir(s.rootdir + hash, os.ModeDir | os.ModePerm)
            if err != nil {
                return ind, err
            }
        } else {
            return ind, err
        }
        ind.Url = url
        ind.Hash = hash
        s.saveIndex(ind, url)
    } else {
        dat, err := ioutil.ReadAll(index)
        if err != nil {
            return ind, err
        }
        json.Unmarshal(dat, &ind)
        _ = index.Close()
    }

    if ind.Url != url {
        return ind, errors.New("hash collisions not implemented")
        // collision, it should forward us to a new one
        //if Index.Get("others").Contains(url) {
        //    hash = Index.Get("others").Get(url)
        //    index, err = os.Open(s.rootdir + hash + "/index.json")
        //    // FIXME jsonize
        //    if Index.Get("url") != url {
        //        // no, there is not a chain of collisions
        //        return nil, errors.new("couldn't find url")
        //    }
        //} else {
            return ind, errors.New("couldn't find url")
        //}
    }
    return ind, nil
}

/*
func (s *Store) Get(url string, start int, end int) ([]xml.Node, error) {
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
    if end > index.Get("count") {
        return nil, errors.new("invalid range")
    }

    ret := make([]xml.Node, end - start)

    i := start
    for nxml := start / 10; nxml <= end / 10; nxml++ {
        fname := rootdir + index.Get("hash") + "/" + strconv.Itoa(nxml) + ".xml"
        f, err := os.Open(fname)
        if err != nil {
            return nil, err
        }
        items := gokogiri.ParseXml(f.Bytes()).Root().Search("//item")
        for ; i < end && i < 10 * (nxml + 1); i++ {
            ret[i] = items[i % 10].Copy()
        }
        f.Close()
    }
    return ret, nil
}
*/

func (s *Store) NumItems(url string) int {
    ind, err := s.indexFor(url)
    if err != nil {
        return 0
    }
    return ind.Count
}

func (s *Store) saveIndex(index Index, url string) error {
    if index.Url != url {
        return errors.New("hash collisions not implemented")
    }
    hash := key(url)
    if index.Hash == "" {
        if _, err := os.Stat(s.rootdir + hash); os.IsNotExist(err) {
            os.Mkdir(s.rootdir + hash, os.ModeDir)
            index.Hash = hash
        }
    }

    ser, err := json.Marshal(index)
    if err != nil {
        return err
    }

    f, err := os.Create(s.rootdir + hash + "/index.json")
    if err != nil {
        return err
    }

    f.Write(ser)
    f.Close()
    return nil
}

func (s *Store) Update(url string, items []xml.Node) error {
    // items must be passed in oldest first
    ind, err := s.indexFor(url)
    if err != nil {
        return err
    }
    lastind := ind.Count - 1
    storefile, err := os.OpenFile(fileof(s, ind, lastind),
                                  os.O_APPEND, os.ModePerm)
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
        gtag, err := it.Search("guid")
        if err != nil {
            return err
        }
        g := gtag[0].Content()
        if _, found := guids[g]; found {
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
        guids[g] = true
    }
    ind.Guids = make([]string, len(guids))
    i := 0
    for g, _ := range guids {
        ind.Guids[i] = g
        i++
    }
    storefile.Close()
    ind.Count = lastind + 1
    err = s.saveIndex(ind, ind.Url)
    if err != nil {
        return err
    }
    return nil
}
