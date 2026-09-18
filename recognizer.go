//go:build windows

package main

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "runtime"
 "strconv"
 "strings"
)

type RecSegment struct { StartMS int64; EndMS int64; Text string }
type whisperJSON struct {
 Transcription []struct {
  Offsets struct { From int64 `json:"from"`; To int64 `json:"to"` } `json:"offsets"`
  Text string `json:"text"`
 } `json:"transcription"`
}
const modelSHA256="422f1ae452ade6f30a004d7e5c6a43195e4433bc370bf23fac9cc591f01a8898"
var progressRe=regexp.MustCompile(`(?i)progress[^0-9]*(\d{1,3})\s*%`)

type recognitionProgressWriter struct { tail diagnosticTail; pending string }
func(w *recognitionProgressWriter)Write(p []byte)(int,error){
 w.tail.Write(p)
 w.pending+=strings.ReplaceAll(string(p),"\r","\n")
 for {
  i:=strings.IndexByte(w.pending,'\n');if i<0{break}
  line:=w.pending[:i];w.pending=w.pending[i+1:]
  if m:=progressRe.FindStringSubmatch(line);len(m)==2{
   n,_:=strconv.Atoi(m[1]);if n>100{n=100}
   updateStatus(25+n*45/100,"正在识别配音…",false,"")
  }
 }
 if len(w.pending)>16384{w.pending=w.pending[len(w.pending)-4096:]}
 return len(p),nil
}

func recognizeAudio(audio,root string)([]RecSegment,error){return recognizeWithLanguage(audio,root,"zh")}

func recognizeWithLanguage(audio,root,lang string)([]RecSegment,error){
 if err:=checkTaskOpen();err!=nil{return nil,err}
 whisper,err:=findRecursive(filepath.Join(root,"whisper"),"whisper-cli.exe")
 if err!=nil{return nil,err}
 ffmpeg,err:=componentPath(root,"ffmpeg.exe");if err!=nil{return nil,err}
 model,err:=componentPath(root,"ggml-base-q5_1.bin");if err!=nil{return nil,err}

 work,err:=os.MkdirTemp("","906-caption-");if err!=nil{return nil,err}
 defer os.RemoveAll(work)
 // The user's paths are opened using Go/Windows Unicode APIs. The native CLI
 // receives fixed ASCII-only relative names, even when Windows TEMP is Chinese.
 ext:=strings.ToLower(filepath.Ext(audio))
 switch ext{case ".wav",".mp3",".m4a",".aac",".flac",".ogg",".wma":default:return nil,fmt.Errorf("不支持的配音格式：%s",ext)}
 sourceName:="source"+ext
 updateStatus(18,"正在准备配音…",false,"")
 if err=copyTaskFile(audio,filepath.Join(work,sourceName));err!=nil{return nil,fmt.Errorf("读取配音失败：%w",err)}

 updateStatus(20,"正在标准化音频…",false,"")
 trans:=exec.Command(ffmpeg,"-nostdin","-hide_banner","-loglevel","error","-y","-i",sourceName,"-map","0:a:0","-vn","-ar","16000","-ac","1","-c:a","pcm_s16le","input.wav")
 trans.Dir=work
 var convertLog diagnosticTail
 trans.Stdout=io.Discard;trans.Stderr=&convertLog
 if err=startManaged(trans);err!=nil{return nil,taskFailure("音频预处理",err,convertLog.String())}
 if err=waitManaged(trans);err!=nil{return nil,taskFailure("音频预处理",err,convertLog.String())}
 if st,e:=os.Stat(filepath.Join(work,"input.wav"));e!=nil||st.Size()<44{return nil,fmt.Errorf("音频预处理没有生成有效 WAV，请检查配音文件")}
 _=os.Remove(filepath.Join(work,sourceName))

 updateStatus(23,"正在校验本地识别组件…",false,"")
 stagedModel:=filepath.Join(work,"model.bin")
 if err=os.Link(model,stagedModel);err!=nil{
  if err=copyTaskFile(model,stagedModel);err!=nil{return nil,fmt.Errorf("准备识别模型失败：%w",err)}
 }
 if err=verifyModel(stagedModel);err!=nil{return nil,err}

 threads:=runtime.NumCPU()/2
 if threads<1{threads=1};if threads>4{threads=4}
 updateStatus(25,"正在加载识别组件…",false,"")
 cmd:=exec.Command(whisper,"-m","model.bin","-f","input.wav","-l",lang,"-ng","-oj","-of","recognition","-pp","-t",strconv.Itoa(threads))
 cmd.Dir=work
 var progress recognitionProgressWriter
 cmd.Stdout=io.Discard;cmd.Stderr=&progress
 if err=startManaged(cmd);err!=nil{return nil,taskFailure("配音识别启动",err,progress.tail.String())}
 if err=waitManaged(cmd);err!=nil{return nil,taskFailure("配音识别",err,progress.tail.String())}
 if err=checkTaskOpen();err!=nil{return nil,err}
 b,err:=os.ReadFile(filepath.Join(work,"recognition.json"))
 if err!=nil{return nil,taskFailure("识别结果读取",err,progress.tail.String())}
 return parseRecognition(b)
}

func verifyModel(path string)error{
 f,err:=os.Open(path);if err!=nil{return err};defer f.Close()
 st,err:=f.Stat();if err!=nil{return err}
 if st.Size()!=59707625{return fmt.Errorf("识别模型不完整，请重新完整解压 V1.4.2 软件包")}
 h:=sha256.New();buf:=make([]byte,1024*1024)
 for{
  if err=checkTaskOpen();err!=nil{return err}
  n,e:=f.Read(buf);if n>0{_,_=h.Write(buf[:n])}
  if e==io.EOF{break};if e!=nil{return e}
 }
 if hex.EncodeToString(h.Sum(nil))!=modelSHA256{return fmt.Errorf("识别模型校验不通过，已停止生成。请重新解压原软件包")}
 return nil
}

func parseRecognition(b []byte)([]RecSegment,error){
 var js whisperJSON
 if err:=json.Unmarshal(b,&js);err!=nil{return nil,fmt.Errorf("识别结果解析失败：%w",err)}
 var out []RecSegment
 for _,s:=range js.Transcription{
  text:=strings.TrimSpace(s.Text)
  if text==""||s.Offsets.From<0||s.Offsets.To<=s.Offsets.From{continue}
  out=append(out,RecSegment{StartMS:s.Offsets.From,EndMS:s.Offsets.To,Text:text})
 }
 if len(out)==0{return nil,fmt.Errorf("未识别到有效语音；请确认导入的是含人声的配音")}
 return out,nil
}
