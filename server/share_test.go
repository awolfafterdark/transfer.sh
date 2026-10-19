package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/dutchcoders/transfer.sh/server/storage"
	"github.com/gorilla/mux"
)

// A second storage adapter proves that previewing does not depend on local paths.
type previewMemory struct {
	storage.Storage
	objects map[string][]byte
	readBytes int
	open int
}
type countedPreviewReader struct { io.Reader; store *previewMemory }
func (r *countedPreviewReader) Read(p []byte)(int,error){ n,e:=r.Reader.Read(p);r.store.readBytes+=n;return n,e }
func (r *countedPreviewReader) Close()error {r.store.open--;return nil}
func (m *previewMemory) Get(_ context.Context,token,name string,_ *storage.Range)(io.ReadCloser,uint64,error){
	b,ok:=m.objects[token+"/"+name];if !ok{return nil,0,errors.New("missing")};m.open++;return &countedPreviewReader{Reader:bytes.NewReader(b),store:m},uint64(len(b)),nil
}
func (m *previewMemory) Head(_ context.Context,token,name string)(uint64,error){b,ok:=m.objects[token+"/"+name];if !ok{return 0,errors.New("missing")};return uint64(len(b)),nil}
func (m *previewMemory) Put(_ context.Context,token,name string,r io.Reader,_ string,_ uint64)error {b,e:=io.ReadAll(r);m.objects[token+"/"+name]=b;return e}
func previewFixture(t *testing.T, data []byte, meta metadata)(*Server,*previewMemory){
	t.Helper();b,e:=json.Marshal(meta);if e!=nil{t.Fatal(e)}
	m:=&previewMemory{objects:map[string][]byte{"TOKEN/example.txt":data,"TOKEN/example.txt.metadata":b}}
	return &Server{storage:m},m
}
func renderPreview(s *Server,token,name string)*httptest.ResponseRecorder{
	req:=httptest.NewRequest("GET","https://files.example/view/"+token+"/"+name,nil)
	req=mux.SetURLVars(req,map[string]string{"token":token,"filename":name});w:=httptest.NewRecorder();s.fileViewHandler(w,req);return w
}
func TestPreviewStorageAndBoundedRead(t *testing.T){
	s,m:=previewFixture(t,[]byte(strings.Repeat("x",previewLimit)+"TAIL"),metadata{ContentType:"text/plain",MaxDownloads:-1})
	w:=renderPreview(s,"TOKEN","example.txt")
	if w.Code!=200 || !strings.Contains(w.Body.String(),"Preview limited") || strings.Contains(w.Body.String(),"TAIL"){t.Fatalf("invalid bounded preview: status %d",w.Code)}
	if m.readBytes>previewLimit+2048{t.Fatalf("read too much: %d",m.readBytes)}
	if m.open!=0{t.Fatalf("leaked %d readers",m.open)}
	if !strings.Contains(w.Body.String(),"https://files.example/get/TOKEN/example.txt"){t.Fatal("missing original download URL")}
}
func TestPreviewMissingExpiredAndEscaped(t *testing.T){
	s,m:=previewFixture(t,[]byte("<script>alert(1)</script>"),metadata{ContentType:"text/plain",MaxDownloads:-1})
	w:=renderPreview(s,"TOKEN","example.txt")
	if w.Code!=200 || strings.Contains(w.Body.String(),"<script>alert") || !strings.Contains(w.Body.String(),"&lt;script&gt;"){t.Fatal("source was not escaped")}
	if renderPreview(s,"missing","example.txt").Code!=404{t.Fatal("missing file must return 404")}
	for _,meta:=range []metadata{{MaxDownloads:-1,MaxDate:time.Now().Add(-time.Hour)},{MaxDownloads:1,Downloads:1}}{
		b,_:=json.Marshal(meta);m.objects["TOKEN/example.txt.metadata"]=b
		if renderPreview(s,"TOKEN","example.txt").Code!=404{t.Fatal("expired file must return 404")}
	}
	if m.open!=0{t.Fatal("leaked metadata readers")}
}
func TestPreviewValidation(t *testing.T){
	for _,name:=range []string{"..","a/b","a\\b","x.metadata","a\x00b"}{if validShareFile("TOKEN",name){t.Fatalf("accepted %q",name)}}
	if !validShareFile("aB_123-","file é #?.txt"){t.Fatal("rejected valid name")}
	for _,name:=range []string{"page.html","vector.svg"}{if kind,_:=previewKind(name,"image/png");kind!="text"{t.Fatal("active content must be source")}}
}
