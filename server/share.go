package server

import (
 "bytes"
 "embed"
 "encoding/csv"
 "encoding/json"
 "html/template"
 "io"
 "mime"
 "net/http"
 "os"
 "path"
 "regexp"
 "strings"
 "unicode/utf8"

 "github.com/gorilla/mux"
)

//go:embed web
var shareWeb embed.FS

const previewLimit = 256 * 1024
var shareTemplates = template.Must(template.ParseFS(shareWeb, "web/*.html"))
var shareToken = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// shareURL always escapes path segments independently. Files remain in the existing backend.
func (s *Server) shareURL(r *http.Request, action, token, filename string) string {
 u := getURL(r, s.proxyPort)
 u.RawQuery, u.Fragment, u.RawPath = "", "", ""
 u.Path = path.Join("/", s.proxyPath, action, token) + "/" + filename
 return u.String()
}

func (s *Server) addShareHeaders(w http.ResponseWriter, r *http.Request, token, filename string) {
 w.Header().Add("X-View-URL", s.shareURL(r, "view", token, filename))
 w.Header().Add("X-Direct-URL", s.shareURL(r, "get", token, filename))
}

func sharePageHeaders(w http.ResponseWriter) {
 w.Header().Set("Content-Type", "text/html; charset=utf-8")
 w.Header().Set("Cache-Control", "no-store")
 w.Header().Set("X-Content-Type-Options", "nosniff")
 w.Header().Set("Referrer-Policy", "no-referrer")
 w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self'; media-src 'self'; frame-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
}

func (s *Server) shareAssetHandler(w http.ResponseWriter, r *http.Request) {
 name := strings.TrimPrefix(r.URL.Path, "/")
 // Only the two explicit routes register this handler.
 if s.webPath != "" { http.FileServer(http.Dir(s.webPath)).ServeHTTP(w,r); return }
 data, err := shareWeb.ReadFile("web/"+name)
 if err != nil { http.NotFound(w,r); return }
 w.Header().Set("X-Content-Type-Options","nosniff")
 w.Header().Set("Content-Type",mime.TypeByExtension(path.Ext(name)))
 _, _ = w.Write(data)
}

func (s *Server) shareHomeHandler(w http.ResponseWriter, r *http.Request) {
 sharePageHeaders(w)
 data := struct{ Base string }{Base: strings.TrimRight(s.proxyPath,"/")}
 var page bytes.Buffer
 var err error
 if s.webPath != "" { err = htmlTemplates.ExecuteTemplate(&page,"index.html",data) } else { err = shareTemplates.ExecuteTemplate(&page,"index.html",data) }
 if err != nil { http.Error(w,"Could not render upload page",500); return }
 _, _ = w.Write(page.Bytes())
}

type fileView struct {
 Base, Filename, MIME, Kind, Text, ViewURL, DirectURL, InlineURL, Uploaded, Expires, Language string
 Size uint64
 Truncated, Temporary bool
 Rows [][]string
}

func validShareFile(token, filename string) bool {
 return shareToken.MatchString(token) && len(token)<=256 && filename!="" && filename!="." && filename!=".." &&
 !strings.ContainsAny(filename,"/\\\x00\r\n") && utf8.ValidString(filename) && !strings.HasSuffix(filename,".metadata")
}

func previewKind(filename, contentType string) (string,string) {
 ext := strings.ToLower(path.Ext(filename))
 // Active formats are always source, irrespective of an uploader-supplied MIME type.
 switch ext {
 case ".html", ".htm", ".svg", ".xml", ".xhtml": return "text","markup"
 case ".json": return "json","json"
 case ".csv": return "csv",""
 case ".py": return "text","python"
 case ".js", ".mjs", ".cjs", ".ts": return "text","javascript"
 case ".txt", ".md", ".log", ".yaml", ".yml", ".sh", ".css", ".go": return "text",""
 }
 ct,_,_ := mime.ParseMediaType(contentType)
 switch ct {
 case "image/png","image/jpeg","image/gif","image/webp","image/avif","image/bmp": return "image",""
 case "application/pdf": return "pdf",""
 case "video/mp4","video/webm","video/ogg": return "video",""
 case "audio/mpeg","audio/mp4","audio/ogg","audio/wav","audio/x-wav","audio/webm","audio/flac": return "audio",""
 case "application/json": return "json","json"
 case "text/csv": return "csv",""
 }
 if strings.HasPrefix(ct,"text/") || ct=="image/svg+xml" || ct=="application/xml" || ct=="application/javascript" { return "text","" }
 return "binary",""
}

func (s *Server) fileViewHandler(w http.ResponseWriter, r *http.Request) {
 vars:=mux.Vars(r); token,filename:=vars["token"],vars["filename"]
 if !validShareFile(token,filename) { http.NotFound(w,r); return }
 meta,err:=s.checkMetadata(r.Context(),token,filename,false)
 if err!=nil { http.NotFound(w,r); return }
 size,err:=s.storage.Head(r.Context(),token,filename)
 if err!=nil { http.NotFound(w,r); return }
 kind,language:=previewKind(filename,meta.ContentType)
 if meta.Encrypted { kind="binary" }
 v:=fileView{Base:strings.TrimRight(s.proxyPath,"/"),Filename:filename,MIME:meta.ContentType,Kind:kind,Language:language,Size:size,
 ViewURL:s.shareURL(r,"view",token,filename),DirectURL:s.shareURL(r,"get",token,filename),InlineURL:s.shareURL(r,"inline",token,filename)}
 if v.MIME=="" { v.MIME="application/octet-stream" }
 if !meta.UploadedAt.IsZero() { v.Uploaded=meta.UploadedAt.UTC().Format("2006-01-02 15:04:05 UTC") }
 if !meta.MaxDate.IsZero() { v.Expires=meta.MaxDate.UTC().Format("2006-01-02 15:04:05 UTC") }
 // Explicitly enabled only by the temporary-runner deployment.
 v.Temporary = temporaryHosting()
 if kind=="text" || kind=="json" || kind=="csv" {
  reader,_,err:=s.storage.Get(r.Context(),token,filename,nil)
  if err!=nil { http.NotFound(w,r); return }
  data,err:=io.ReadAll(io.LimitReader(reader,previewLimit+1)); _=reader.Close()
  if err!=nil { http.Error(w,"Could not read preview",500); return }
  v.Truncated=len(data)>previewLimit
  if v.Truncated { data=data[:previewLimit] }
  v.Text=strings.ToValidUTF8(string(data),"�")
  if kind=="json" && !v.Truncated {
   var pretty bytes.Buffer
   if json.Indent(&pretty,data,"","  ")==nil { v.Text=pretty.String() }
  }
  if kind=="csv" {
   csvReader:=csv.NewReader(strings.NewReader(v.Text)); csvReader.FieldsPerRecord=-1
   for i:=0;i<200;i++ {
    row,e:=csvReader.Read(); if e==io.EOF { break }; if e!=nil { v.Kind="text"; v.Rows=nil; break }
    v.Rows=append(v.Rows,row)
   }
   if v.Kind=="csv" { if _,e:=csvReader.Read(); e!=io.EOF { v.Truncated=true } }
  }
 }
 var page bytes.Buffer
 if err:=shareTemplates.ExecuteTemplate(&page,"preview.html",v); err!=nil { http.Error(w,"Could not render preview",500); return }
 sharePageHeaders(w)
 _,_=w.Write(page.Bytes())
}

func temporaryHosting() bool { return os.Getenv("TRANSFER_TEMPORARY_HOSTING")=="true" }
