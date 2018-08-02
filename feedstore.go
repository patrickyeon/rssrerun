package rssrerun

import (
    "crypto/md5"
    "encoding/hex"
    "encoding/json"
    "errors"
    "io/ioutil"
    "os"
    "strconv"
    "time"

    "github.com/patrickyeon/rssrerun/util"
)

//  All of the feeds we monitor will be stored broken up by items already to
// avoid parsing them more often than necessary. To get something started, I'm
// making a hack datastore out of json files and raw xml snippets on disk. I
// expect the interface to be agnostic enough to this backing store that I will
// be able to change to something better later and not break anything.
type Store interface {
	//  A `Store` contains an `Index` for every feed, which needs to be created
    // before it's used. Will error if one already exists.
	CreateIndex(url string) (Index, error)
    //  An array of `Item`s for the feed at url, oldest first, of length
    // (`end` - `start`)
	Get(url string, start int, end int) ([]Item, error)
    // How many `Item`s stored for `url`?
	NumItems(url string) int
    // add `items` to the `Index` for `url`. They must be passed in oldest first
	Update(url string, items []Item) error
    // Getter for general-purpose metadata
	GetInfo(url string, key string) (string, error)
    // Setter for general-purpose metadata
	SetInfo(url string, key string, val string) error
    // Return a struct that satisfies the `Feed` interface but is backed by us
    FeedFor(url string, ds *DateSource) (Feed, error)
    // Check if we have a url stored
    Contains(url string) bool
    // List out the urls we have stored
    List() []string
}

/* The `jsonStore` is a directory, with subdirectories that are the GUID for the
  relevant `Index`. Each subdirectory has:
/index.json (info)
/offsets.json (where in files items are
/0.xml (items 0-9)
/1.xml (items 1-19)
 [...]
/n.xml (items n*10 - max)
 items are stored as <item> elements, oldest first
*/

type jsonStore struct {
    // root of the `jsonStore`
    rootdir string
    // function to convert a url to a subdirectory name
    key func(string)string
    // function to canonicalize a url
    canon func(string) (string, error)
}

//  An `Index` holds information for a specific feed.
/* index json obj:
{'url': 'actual url used',
 'count': 'number of items',
 'hash': 'actual hash',
 'others': {$url: $hash}, // as in, other urls that have collided with this hash
 'guids': [set_of_stashed_guids], // guids for items we've got
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
    offsets map[string]int64
}

func NewJSONStore(dir string) *jsonStore {
    // expand dir to canonical rep
    // make sure it exists
    ret := new(jsonStore)
    ret.rootdir = dir
    ret.key = justmd5
    ret.canon = cachingFollowHttp
    return ret
}

//  create the key for an `url` by MD5'ing it. Eventually this will end up with
// a collision, and that's handled by the `Index`.
func justmd5(url string) string {
    ret := md5.Sum([]byte(url))
    return hex.EncodeToString(ret[:])
}

//  wrap a cache of url->canonicalization mappings around the canonicalization
// so that we're not doing a fetch every time we want the canonical
// url for something.
var canonCache = make(map[string]string)
func cachingFollowHttp(url string) (string, error) {
    if canonCache[url] != "" {
        return canonCache[url], nil
    }
    ret, err := util.CanonicalUrl(url)
    if err == nil {
        canonCache[url] = ret
    }
    return ret, err
}

func fileof(s *jsonStore, ind Index, item int) string {
    retval := s.rootdir + ind.Hash + "/"
    if item == -1 {
        // special case, index
        retval += "index.json"
    } else if item == -2 {
        // another special case, offsets
        retval += "offsets.json"
    } else if item >= 0 {
        retval += strconv.Itoa(item / 10) + ".xml"
    }
    return retval
}

func (s *jsonStore) indexFor(url string) (Index, error) {
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

    offsets, err := os.Open(fileof(s, ind, -2))
    if err != nil {
        return Index{}, err
    }
    dat, err := ioutil.ReadAll(offsets)
    if err != nil {
        return Index{}, err
    }
    err = json.Unmarshal(dat, &(ind.offsets))
    if err != nil {
        return Index{}, err
    }

    return ind, nil
}

func (s *jsonStore) indexForHash(hash string) (Index, error) {
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
    index.Close()
    offsets, err := os.Open(fileof(s, ind, -2))
    if err != nil {
        return Index{}, err
    }
    dat, err = ioutil.ReadAll(offsets)
    if err != nil {
        return Index{}, err
    }
    if err = json.Unmarshal(dat, &(ind.offsets)); err != nil {
        return Index{}, err
    }

    return ind, nil
}

func (s *jsonStore) Contains(url string) bool {
    _, err := s.indexFor(url)
    if err != nil {
        return false
    }
    return true
}

func (s *jsonStore) List() []string {
    root, err := os.Open(s.rootdir)
    if err != nil {
        return nil
    }
    defer root.Close()
    hashes, err := root.Readdirnames(0)
    if err != nil {
        return nil
    }

    retval := []string{}
    for _, hash := range hashes {
        ind, err := s.indexForHash(hash)
        if err != nil {
            continue
        }
        retval = append(retval, ind.Url)
    }
    return retval
}

func (s *jsonStore) CreateIndex(url string) (Index, error) {
    url, err := s.canon(url)
    if err != nil {
        return Index{}, err
    }
    if s.Contains(url) {
        return Index{}, errors.New("Index already exists")
    }

    ind := Index{}
    ind.Url = url
    hash := s.key(url)
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
    ind.offsets = make(map[string]int64, 0)
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

func (s *jsonStore) Get(url string, start int, end int) ([]Item, error) {
    index, err := s.indexFor(url)
    if err != nil {
        return nil, err
    }
    return s.getInd(index, start, end)
}

func (s *jsonStore) getInd(index Index, start int, end int) ([]Item, error) {
    if start < 0 || end <= start {
        return nil, errors.New("invalid range")
    }
    if end > index.Count {
        return nil, errors.New("invalid range")
    }

    ret := make([]Item, end - start)
    var ftxt []byte

    fname := ""
    for i := start; i < end; i++ {
        if fname != fileof(s, index, i) {
            fname = fileof(s, index, i)
            f, err := os.Open(fname)
            if err != nil {
                return nil, err
            }
            ftxt, err = ioutil.ReadAll(f)
            f.Close()
            if err != nil {
                return nil, err
            }
        }
        endbyte := index.offsets[strconv.Itoa(i + 1)]
        if endbyte == 0 {
            endbyte = int64(len(ftxt))
        }
        // ignore the newline we added when storing in Update()
        itemBytes := ftxt[index.offsets[strconv.Itoa(i)] : endbyte - 1]
        retval, err := MkItem(itemBytes)
        if err != nil {
            return nil, err
        }
        ret[i - start] = retval
    }
    return ret, nil
}

func (s *jsonStore) NumItems(url string) int {
    ind, err := s.indexFor(url)
    if err != nil {
        return 0
    }
    return s.numItems(ind)
}

func (s *jsonStore) numItems(idx Index) int {
    return idx.Count
}

func (s *jsonStore) saveIndex(index Index) error {
    serind, err := json.Marshal(index)
    if err != nil {
        return err
    }
    offind, err := json.Marshal(index.offsets)
    if err != nil {
        return err
    }

    f, err := os.Create(s.rootdir + index.Hash + "/index.json")
    if err != nil {
        return err
    }

    f.Write(serind)
    f.Close()
    f, err = os.Create(s.rootdir + index.Hash + "/offsets.json")
    if err != nil {
        return err
    }
    f.Write(offind)
    f.Close()
    return nil
}

func (s *jsonStore) Update(url string, items []Item) error {
    // items must be passed in oldest first
    ind, err := s.indexFor(url)
    if err != nil {
        return err
    }
    // FIXME will this lead to trying to open the index? Why doesn't it?
    lastind := ind.Count - 1
    _, err = os.OpenFile(fileof(s, ind, -2),
                                   os.O_APPEND | os.O_WRONLY, os.ModePerm)
    if err != nil {
        return err
    }
    storefile, err := os.OpenFile(fileof(s, ind, lastind),
                                  os.O_APPEND | os.O_WRONLY, os.ModePerm)
    if os.IsNotExist(err) {
        storefile, err = os.Create(fileof(s, ind, lastind))
    }
    if err != nil {
        return err
    }

    stat, _ := storefile.Stat()
    curPos := stat.Size()
    // keep track of guids in a set
    guids := make(map[string]bool)
    for _, g := range ind.Guids {
        guids[g] = true
    }

    for _, it := range items {
        guid, err := it.Guid()
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
            curPos = 0
        }
        nWritten, err := storefile.WriteString(it.String() + "\n")
        if err != nil {
            storefile.Close()
            return err
        }
        guids[guid] = true
        ind.offsets[strconv.Itoa(lastind)] = curPos
        curPos += int64(nWritten)
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

func (s *jsonStore) GetInfo(url string, key string) (string, error) {
    ind, err := s.indexFor(url)
    if err != nil {
        return "", err
    }
    return s.getInfo(ind, key), nil
}

func (s *jsonStore) getInfo(idx Index, key string) string {
    return idx.Meta[key]
}

func (s *jsonStore) SetInfo(url string, key string, val string) error {
    ind, err := s.indexFor(url)
    if err == nil {
        err = s.setInfo(ind, key, val)
    }
    return err
}

func (s *jsonStore) setInfo(idx Index, key string, val string) error {
    if idx.Meta == nil {
        idx.Meta = make(map[string]string)
    }
    idx.Meta[key] = val
    return s.saveIndex(idx)
}

func (s *jsonStore) FeedFor(url string, ds *DateSource) (Feed, error) {
    idx, err := s.indexFor(url)
    if err != nil {
        return nil, err
    }
    wrap := s.getInfo(idx, "wrapper")
    feed, err := NewFeed([]byte(wrap), nil)
    if err != nil {
        return nil, err
    }
    return &StoredFeed{feed, ds, idx, s}, nil
}

type StoredFeed struct {
    feed Feed
    ds *DateSource
    idx Index
    store *jsonStore
}

//  Some of the `StoredFeed` functions will just be pass-through to the
// underlying `Feed` interface.

func (f *StoredFeed) Wrapper() []byte {
    return f.feed.Wrapper()
}
func (f *StoredFeed) BytesWithItems(items []Item) []byte {
    return f.feed.BytesWithItems(items)
}

//  Other functions need a bit more thought

func (f *StoredFeed) Items(start, end int) []Item {
    ret, err := f.store.getInd(f.idx, start, end)
    if err != nil {
        panic(err.Error())
    }
    return ret
}

func (f *StoredFeed) Item(idx int) Item {
    ret, err := f.store.getInd(f.idx, idx, idx + 1)
    if err != nil {
        panic(err.Error())
    }
    return ret[0]
}

func (f *StoredFeed) LenItems() int {
    return f.idx.Count
}

func (f *StoredFeed) allItems() []Item {
    return f.Items(0, f.LenItems() - 1)
}

func (f *StoredFeed) appendItems(items []Item) {
    // I don't know that I want this to be available
    panic("StoredFeed represents a read-only interface." +
          " Append to the underlying store instead.")
}

func (f *StoredFeed) ShiftedAt(n int, t time.Time) ([]Item, error) {
    return univShiftedAt(n, t, f, f.ds)
}
