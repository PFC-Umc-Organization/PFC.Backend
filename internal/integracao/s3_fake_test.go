package integracao

import (
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type fakeObject struct {
	data        []byte
	contentType string
}

// fakeS3 answers path-style requests (/<bucket>/<key>) and the browser's
// form POST, enforcing the policy's Content-Type and size like S3 does.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string]fakeObject
}

func newFakeS3() *fakeS3 { return &fakeS3{objects: map[string]fakeObject{}} }

func (f *fakeS3) serve(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", u.Host)
	if err != nil {
		return err
	}
	go http.Serve(ln, f)
	return nil
}

func (f *fakeS3) has(key string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects[key]
	return ok
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	switch {
	case r.Method == http.MethodPost && r.URL.Query().Has("delete"):
		f.deleteObjects(w, r)
		return
	case r.Method == http.MethodPost:
		f.post(w, r)
		return
	case r.Method == http.MethodGet && key == "" && r.URL.Query().Get("list-type") == "2":
		f.list(w, r.URL.Query().Get("prefix"))
		return
	}
	obj, ok := f.objects[key]
	switch r.Method {
	case http.MethodHead, http.MethodGet:
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", obj.contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(obj.data)))
		if d := r.URL.Query().Get("response-content-disposition"); d != "" {
			w.Header().Set("Content-Disposition", d)
		}
		if r.Method == http.MethodGet {
			w.Write(obj.data)
		}
	case http.MethodDelete:
		delete(f.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (f *fakeS3) post(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(1 << 20); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "sem arquivo", http.StatusBadRequest)
		return
	}
	data, _ := io.ReadAll(file)
	key, contentType := r.FormValue("key"), r.FormValue("Content-Type")

	raw, _ := base64.StdEncoding.DecodeString(r.FormValue("policy"))
	var policy struct {
		Conditions []json.RawMessage `json:"conditions"`
	}
	if json.Unmarshal(raw, &policy) != nil || r.FormValue("X-Amz-Signature") == "" {
		http.Error(w, "policy inválida", http.StatusForbidden)
		return
	}
	for _, c := range policy.Conditions {
		var rule []any
		if json.Unmarshal(c, &rule) != nil {
			var exact map[string]string
			json.Unmarshal(c, &exact)
			if want, ok := exact["key"]; ok && want != key {
				http.Error(w, "key fora da policy", http.StatusForbidden)
				return
			}
			continue
		}
		switch rule[0] {
		case "eq":
			if rule[1] == "$Content-Type" && rule[2] != contentType {
				http.Error(w, "Content-Type fora da policy", http.StatusForbidden)
				return
			}
		case "content-length-range":
			if float64(len(data)) < rule[1].(float64) || float64(len(data)) > rule[2].(float64) {
				http.Error(w, "EntityTooLarge", http.StatusBadRequest)
				return
			}
		}
	}
	f.objects[key] = fakeObject{data: data, contentType: contentType}
	w.WriteHeader(http.StatusNoContent)
}

// list answers ListObjectsV2 in a single page.
func (f *fakeS3) list(w http.ResponseWriter, prefix string) {
	type object struct{ Key string }
	out := struct {
		XMLName  xml.Name `xml:"ListBucketResult"`
		Contents []object
	}{}
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			out.Contents = append(out.Contents, object{k})
		}
	}
	sort.Slice(out.Contents, func(i, j int) bool { return out.Contents[i].Key < out.Contents[j].Key })
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(out)
}

func (f *fakeS3) deleteObjects(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Object []struct{ Key string }
	}
	if err := xml.NewDecoder(r.Body).Decode(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	for _, o := range in.Object {
		delete(f.objects, o.Key)
	}
	w.Header().Set("Content-Type", "application/xml")
	io.WriteString(w, "<DeleteResult></DeleteResult>")
}
