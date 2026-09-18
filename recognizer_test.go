//go:build windows

package main

import (
 "archive/zip"
 "encoding/xml"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "testing"
 "time"
)

func TestRecognitionOffsets(t *testing.T){
 s,e:=parseRecognition([]byte(`{"transcription":[{"offsets":{"from":1250,"to":3560},"text":"原文"},{"offsets":{"from":3560,"to":3560},"text":""}]}`))
 if e!=nil||len(s)!=1||s[0].StartMS!=1250||s[0].EndMS!=3560{t.Fatalf("bad offsets: %+v %v",s,e)}
 if _,e=parseRecognition([]byte(`{"transcription":[]}`));e==nil{t.Fatal("empty audio result accepted")}
}
func TestProgressParsing(t *testing.T){
 w:=&recognitionProgressWriter{}
 w.Write([]byte("whisper_print_progress_callback: prog"))
 w.Write([]byte("ress = 50%\r\n"))
 uiMu.Lock();p:=taskProgress;uiMu.Unlock()
 if p!=47{t.Fatalf("progress = %d",p)}
}
func TestTXTAndDOCX(t *testing.T){
 dir:=t.TempDir();expected:="测试文稿：甲乙丙丁。"
 p:=filepath.Join(dir,"稿件.txt");os.WriteFile(p,[]byte(expected),0600)
 got,e:=readScript(p);if e!=nil||got!=expected{t.Fatalf("TXT %q %v",got,e)}
 p=filepath.Join(dir,"稿件.docx");f,e:=os.Create(p);if e!=nil{t.Fatal(e)}
 z:=zip.NewWriter(f);w,e:=z.Create("word/document.xml");if e!=nil{t.Fatal(e)}
 io.WriteString(w,`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>`+expected+`</w:t></w:r></w:p></w:body></w:document>`)
 z.Close();f.Close();got,e=readScript(p);if e!=nil||got!=expected{t.Fatalf("DOCX %q %v",got,e)}
}
func TestOneSelectedExport(t *testing.T){
 subs:=[]Subtitle{{StartMS:200,EndMS:1500,Text:"甲乙 & 丙丁。"},{StartMS:1700,EndMS:2800,Text:"第二句。"}}
 suffixes:=[]string{".fcpxml",".itt",".srt",".xml","_透明字幕PNG",".csv"}
 for format,suffix:=range suffixes{
  t.Run(suffix,func(t *testing.T){
   dir:=t.TempDir();out,e:=exportResult(subs,filepath.Join(dir,"测试配音.wav"),format)
   if e!=nil{t.Fatal(e)}
   entries,e:=os.ReadDir(dir);if e!=nil||len(entries)!=1{t.Fatalf("not single-format: %v %v",entries,e)}
   if !strings.HasSuffix(out,suffix){t.Fatalf("wrong selected format: %s",out)}
   if format==0||format==1||format==3{
    f,e:=os.Open(out);if e!=nil{t.Fatal(e)};defer f.Close()
    dec:=xml.NewDecoder(f)
    for{_,e=dec.Token();if e==io.EOF{break};if e!=nil{t.Fatal(e)}}
   }
   if format==2{
    b,_:=os.ReadFile(out)
    if !strings.Contains(string(b),subs[0].Text)||!strings.Contains(string(b),"00:00:00,200 --> 00:00:01,500"){t.Fatal("SRT text/timing modified")}
   }
  })
 }
}

// Integration fixture is the upstream public JFK recording, not user material.
// Infer with its true language, and separately exercise default zh below.
func TestWindowsAudioPipeline(t *testing.T){
 root:=os.Getenv("SUBTITLE_TEST_RUNTIME");sample:=os.Getenv("SUBTITLE_TEST_AUDIO")
 if root==""||sample==""{t.Skip("offline fixtures not configured")}
 dir:=filepath.Join(t.TempDir(),"中文 路径 测试")
 if e:=os.MkdirAll(dir,0700);e!=nil{t.Fatal(e)}
 tools:=filepath.Join(dir,"软件 中文 runtime")
 if e:=os.MkdirAll(filepath.Join(tools,"whisper"),0700);e!=nil{t.Fatal(e)}
 for _,name:=range []string{"whisper/whisper-cli.exe","ffmpeg.exe","ggml-base-q5_1.bin"}{
  src:=filepath.Join(root,filepath.FromSlash(name));dst:=filepath.Join(tools,filepath.FromSlash(name))
  if e:=os.Link(src,dst);e!=nil{if e=copyTaskFile(src,dst);e!=nil{t.Fatal(e)}}
 }
 // Test with no compiler, MSVC redistributable or third-party DLL directory on PATH.
 t.Setenv("PATH",filepath.Join(os.Getenv("SystemRoot"),"System32"))
 t.Setenv("TEMP",dir);t.Setenv("TMP",dir)
 if e:=copyTaskFile(sample,filepath.Join(dir,"source.wav"));e!=nil{t.Fatal(e)}
 ff:=filepath.Join(tools,"ffmpeg.exe")
 for _,ext:=range []string{"mp3","m4a"}{
  t.Run(ext,func(t *testing.T){
   enc:=exec.Command(ff,"-nostdin","-hide_banner","-loglevel","error","-y","-i","source.wav","-ar","48000","-ac","2","encoded."+ext)
   enc.Dir=dir
   if b,e:=enc.CombinedOutput();e!=nil{t.Fatalf("fixture encoding: %v %s",e,b)}
   input:=filepath.Join(dir,"配音 文件（测试）."+ext)
   if e:=os.Rename(filepath.Join(dir,"encoded."+ext),input);e!=nil{t.Fatal(e)}
   before:=time.Now()
   segments,e:=recognizeWithLanguage(input,tools,"en")
   if e!=nil{t.Fatal(e)}
   if len(segments)==0{t.Fatal("no segments")}
   var text strings.Builder
   for _,s:=range segments{text.WriteString(s.Text)}
   if !strings.Contains(strings.ToLower(text.String()),"country"){t.Fatalf("inference failed known phrase: %s",text.String())}
   t.Logf("%s inference OK: %d segments, %s",ext,len(segments),time.Since(before))
   subs:=alignScript([]string{"And so my fellow Americans.","Ask not what your country can do for you."},segments)
   targetDir:=t.TempDir()
   out,e:=exportResult(subs,filepath.Join(targetDir,"测试.wav"),2);if e!=nil{t.Fatal(e)}
   if st,e:=os.Stat(out);e!=nil||st.Size()==0{t.Fatalf("SRT failed: %v",e)}
   left,_:=filepath.Glob(filepath.Join(dir,"906-caption-*"));if len(left)!=0{t.Fatalf("temporary work not cleaned: %v",left)}
   processMu.Lock();active:=currentCmd!=nil;processMu.Unlock();if active{t.Fatal("child process registration left behind")}
  })
 }
}

func TestInvalidAudioFailsWithoutCrash(t *testing.T){
 root:=os.Getenv("SUBTITLE_TEST_RUNTIME");if root==""{t.Skip("offline fixtures not configured")}
 p:=filepath.Join(t.TempDir(),"坏的音频.mp3");os.WriteFile(p,[]byte("not an audio file"),0600)
 _,e:=recognizeAudio(p,root)
 if e==nil||!strings.Contains(e.Error(),"音频预处理"){t.Fatalf("bad error: %v",e)}
 // The log is generated on the test runner only and is not distributed.
 _=os.Remove(filepath.Join(appDir(),"906_subtitle_error.log"))
}
