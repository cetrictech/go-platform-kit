package s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConfigurationRejectsUnsafeOrUnboundedSettings(t *testing.T) {
	cases := []Config{
		{},
		{Bucket: "assets", Region: "us-east-1"},
		{Bucket: "assets", Region: "us-east-1", Credentials: StaticCredentials("", "secret", "")},
		{Bucket: "assets", Region: "us-east-1", Credentials: StaticCredentials("key", "secret", ""), Endpoint: "http://127.0.0.1:1"},
		{Bucket: "assets", Region: "us-east-1", Credentials: StaticCredentials("key", "secret", ""), AllowPlaintext: true, Endpoint: "http://127.0.0.1:1", PartSize: 1024},
		{Bucket: "assets", Region: "us-east-1", Credentials: StaticCredentials("key", "secret", ""), AllowPlaintext: true, Endpoint: "http://127.0.0.1:1", Concurrency: 9},
		{Bucket: "assets", Region: "us-east-1", Credentials: StaticCredentials("key", "secret", ""), AllowPlaintext: true, Endpoint: "http://127.0.0.1:1", MaxPresign: 8 * 24 * time.Hour},
		{Bucket: "a/b", Region: "us-east-1", Credentials: StaticCredentials("key", "secret", "")},
	}
	for _, configuration := range cases {
		client, err := New(context.Background(), configuration)
		if client != nil || err == nil {
			if client != nil {
				client.Close()
			}
			t.Fatalf("accepted invalid configuration: %+v (%v)", configuration, err)
		}
	}
}

func TestObjectRoundTripMultipartCancellationAndPresignBounds(t *testing.T) {
	store := newFake()
	server := httptest.NewServer(store)
	defer server.Close()
	defer close(store.release)
	client, err := New(context.Background(), Config{
		Bucket: "assets", Region: "us-east-1", Endpoint: server.URL, AllowPlaintext: true,
		Credentials: StaticCredentials("access", "secret", "session"), AllowSmallParts: true, PartSize: 64 << 10, MaxPresign: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	payload := []byte("alpha-object")
	metadata := map[string]string{"owner": "library"}
	if err := client.Put(context.Background(), "folder/file.txt", bytes.NewReader(payload), PutOptions{ContentType: "text/plain", Metadata: metadata, ServerSideEncryption: "AES256"}); err != nil {
		t.Fatalf("put: %v\ntrace:\n%s", err, store.trace())
	}
	if !store.sawChecksum || !store.sawEncryption {
		t.Fatalf("checksum=%v encryption=%v\n%s", store.sawChecksum, store.sawEncryption, store.trace())
	}
	object, err := client.Get(context.Background(), "folder/file.txt")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(object.Body)
	closeErr := object.Body.Close()
	if err != nil || closeErr != nil {
		t.Fatal(err, closeErr)
	}
	if !bytes.Equal(body, payload) || object.ContentType != "text/plain" || object.Metadata["owner"] != "library" || object.Size != int64(len(payload)) || object.ChecksumSHA256 == "" {
		t.Fatalf("object mismatch: %+v body=%q", object.Info, body)
	}
	head, err := client.Head(context.Background(), "folder/file.txt")
	if err != nil || head.Size != object.Size || head.ChecksumSHA256 != object.ChecksumSHA256 {
		t.Fatalf("head: %+v %v", head, err)
	}
	if _, err := client.Get(context.Background(), "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing get: %v", err)
	}
	if _, err := client.PresignGet(context.Background(), "folder/file.txt", 2*time.Minute); err == nil {
		t.Fatal("presign exceeded the client bound")
	}
	signed, err := client.PresignGet(context.Background(), "folder/file.txt", 30*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(signed, "X-Amz-Expires=30") || !strings.Contains(signed, "X-Amz-Signature=") {
		t.Fatalf("presign URL is not a bounded signed request: %s", signed)
	}
	response, err := http.Get(signed)
	if err != nil {
		t.Fatal(err)
	}
	signedBody, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK || !bytes.Equal(signedBody, payload) {
		t.Fatalf("presigned get status=%d body=%q", response.StatusCode, signedBody)
	}
	putURL, err := client.PresignPut(context.Background(), "direct.txt", 30*time.Second)
	if err != nil || !strings.Contains(putURL, "X-Amz-Expires=30") {
		t.Fatal(err, putURL)
	}
	large := bytes.Repeat([]byte("m"), 70<<10)
	if err := client.Put(context.Background(), "large.bin", readerOnly{bytes.NewReader(large)}, PutOptions{ContentType: "application/octet-stream"}); err != nil {
		t.Fatalf("multipart put: %v\n%s", err, store.trace())
	}
	if !store.completedMultipart {
		t.Fatal("large object bypassed multipart upload")
	}
	largeObject, err := client.Get(context.Background(), "large.bin")
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(largeObject.Body)
	largeObject.Body.Close()
	if err != nil || !bytes.Equal(got, large) {
		t.Fatalf("multipart body mismatch: %v len=%d", err, len(got))
	}
	if err := client.Delete(context.Background(), "large.bin"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Head(context.Background(), "large.bin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted object remained: %v", err)
	}

	store.blockParts = true
	ctx, cancel := context.WithCancel(context.Background())
	uploaded := make(chan error, 1)
	go func() {
		uploaded <- client.Put(ctx, "canceled.bin", readerOnly{bytes.NewReader(large)}, PutOptions{})
	}()
	select {
	case <-store.partStarted:
	case err := <-uploaded:
		t.Fatalf("upload finished before a part started: %v\n%s", err, store.trace())
	case <-time.After(3 * time.Second):
		t.Fatalf("multipart part did not start\n%s", store.trace())
	}
	cancel()
	select {
	case err := <-uploaded:
		if err == nil {
			t.Fatal("canceled upload succeeded")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("canceled upload did not return")
	}
	if !store.aborted() {
		t.Fatalf("canceled multipart upload was not aborted\n%s", store.trace())
	}
}

type readerOnly struct{ io.Reader }

type fakeStore struct {
	mu                 sync.Mutex
	objects            map[string]stored
	uploads            map[string]*upload
	requests           []string
	sawChecksum        bool
	sawEncryption      bool
	completedMultipart bool
	blockParts         bool
	partStarted        chan struct{}
	release            chan struct{}
	once               sync.Once
	abortCount         int
}

type stored struct {
	body        []byte
	contentType string
	metadata    map[string]string
	checksum    string
}

type upload struct {
	contentType string
	metadata    map[string]string
	parts       map[int][]byte
}

func newFake() *fakeStore {
	return &fakeStore{objects: map[string]stored{}, uploads: map[string]*upload{}, partStarted: make(chan struct{}), release: make(chan struct{})}
}

func (f *fakeStore) trace() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.requests, "\n")
}

func (f *fakeStore) aborted() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.abortCount > 0
}

func (f *fakeStore) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.Method+" "+r.URL.RequestURI())
	f.mu.Unlock()
	key, bucket := objectKey(r.URL.Path)
	if !bucket {
		http.NotFound(w, r)
		return
	}
	query := r.URL.Query()
	switch {
	case key == "" && r.Method == http.MethodHead:
		w.WriteHeader(http.StatusOK)
	case query.Has("uploads") && r.Method == http.MethodPost:
		f.createUpload(w, r, key)
	case query.Get("uploadId") != "" && query.Get("partNumber") != "" && r.Method == http.MethodPut:
		f.savePart(w, r, query.Get("uploadId"), query.Get("partNumber"))
	case query.Get("uploadId") != "" && r.Method == http.MethodPost:
		f.complete(w, r, key, query.Get("uploadId"))
	case query.Get("uploadId") != "" && r.Method == http.MethodDelete:
		f.abort(w, query.Get("uploadId"))
	case r.Method == http.MethodPut:
		f.put(w, r, key)
	case r.Method == http.MethodGet:
		f.get(w, r, key, true)
	case r.Method == http.MethodHead:
		f.get(w, r, key, false)
	case r.Method == http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unexpected "+r.Method+" "+r.URL.RequestURI(), http.StatusNotImplemented)
	}
}

func objectKey(path string) (string, bool) {
	path = strings.TrimPrefix(path, "/")
	if path == "assets" {
		return "", true
	}
	if !strings.HasPrefix(path, "assets/") {
		return "", false
	}
	return strings.TrimPrefix(path, "assets/"), true
}

func (f *fakeStore) put(w http.ResponseWriter, r *http.Request, key string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.noteChecksum(r)
	checksum := f.store(key, body, r.Header.Get("Content-Type"), metadataFrom(r.Header), r.Header.Get("x-amz-server-side-encryption") == "AES256")
	w.Header().Set("ETag", `"object"`)
	w.Header().Set("x-amz-checksum-sha256", checksum)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeStore) noteChecksum(r *http.Request) {
	if r.Header.Get("x-amz-checksum-sha256") == "" && r.Trailer.Get("x-amz-checksum-sha256") == "" && r.Header.Get("x-amz-sdk-checksum-algorithm") != "SHA256" {
		return
	}
	f.mu.Lock()
	f.sawChecksum = true
	f.mu.Unlock()
}

func (f *fakeStore) store(key string, body []byte, contentType string, metadata map[string]string, encrypted bool) string {
	sum := sha256.Sum256(body)
	checksum := base64.StdEncoding.EncodeToString(sum[:])
	f.mu.Lock()
	f.objects[key] = stored{body: append([]byte(nil), body...), contentType: contentType, metadata: metadata, checksum: checksum}
	if encrypted {
		f.sawEncryption = true
	}
	f.mu.Unlock()
	return checksum
}

func metadataFrom(header http.Header) map[string]string {
	out := map[string]string{}
	for key, values := range header {
		lower := strings.ToLower(key)
		if strings.HasPrefix(lower, "x-amz-meta-") && len(values) > 0 {
			out[strings.TrimPrefix(lower, "x-amz-meta-")] = values[0]
		}
	}
	return out
}

func (f *fakeStore) get(w http.ResponseWriter, r *http.Request, key string, withBody bool) {
	f.mu.Lock()
	item, ok := f.objects[key]
	f.mu.Unlock()
	if !ok {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<Error><Code>NoSuchKey</Code><Message>missing</Message></Error>`)
		return
	}
	w.Header().Set("Content-Type", item.contentType)
	w.Header().Set("Content-Length", strconv.Itoa(len(item.body)))
	w.Header().Set("x-amz-checksum-sha256", item.checksum)
	for key, value := range item.metadata {
		w.Header().Set("x-amz-meta-"+key, value)
	}
	w.WriteHeader(http.StatusOK)
	if withBody {
		_, _ = w.Write(item.body)
	}
}

func (f *fakeStore) createUpload(w http.ResponseWriter, r *http.Request, key string) {
	id := "upload-1"
	if strings.Contains(key, "canceled") {
		id = "upload-cancel"
	}
	f.noteChecksum(r)
	f.mu.Lock()
	f.uploads[id] = &upload{contentType: r.Header.Get("Content-Type"), metadata: metadataFrom(r.Header), parts: map[int][]byte{}}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	_, _ = io.WriteString(w, `<InitiateMultipartUploadResult><Bucket>assets</Bucket><Key>`+key+`</Key><UploadId>`+id+`</UploadId></InitiateMultipartUploadResult>`)
}

func (f *fakeStore) savePart(w http.ResponseWriter, r *http.Request, id, number string) {
	if f.blockParts {
		f.once.Do(func() { close(f.partStarted) })
		select {
		case <-r.Context().Done():
		case <-f.release:
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	part, _ := strconv.Atoi(number)
	f.noteChecksum(r)
	f.mu.Lock()
	upload := f.uploads[id]
	if upload != nil {
		upload.parts[part] = body
	}
	f.mu.Unlock()
	if upload == nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("ETag", `"part"`)
	w.WriteHeader(http.StatusOK)
}

func (f *fakeStore) complete(w http.ResponseWriter, r *http.Request, key, id string) {
	f.mu.Lock()
	upload := f.uploads[id]
	delete(f.uploads, id)
	f.mu.Unlock()
	if upload == nil {
		http.NotFound(w, r)
		return
	}
	var body []byte
	for n := 1; n <= len(upload.parts); n++ {
		part, ok := upload.parts[n]
		if !ok {
			http.Error(w, "missing part", http.StatusBadRequest)
			return
		}
		body = append(body, part...)
	}
	checksum := f.store(key, body, upload.contentType, upload.metadata, false)
	f.mu.Lock()
	f.completedMultipart = true
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	_, _ = io.WriteString(w, `<CompleteMultipartUploadResult><Bucket>assets</Bucket><Key>`+key+`</Key><ETag>"object"</ETag><ChecksumSHA256>`+checksum+`</ChecksumSHA256></CompleteMultipartUploadResult>`)
}

func (f *fakeStore) abort(w http.ResponseWriter, id string) {
	f.mu.Lock()
	delete(f.uploads, id)
	f.abortCount++
	f.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}
