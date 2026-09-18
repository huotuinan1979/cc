//go:build windows

package main

import (
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "sync"
 "syscall"
 "time"
)

const appVersion = "V1.4.2"
var (
 processMu sync.Mutex
 currentCmd *exec.Cmd
 appClosing bool
)

func appDir() string {
 exe, err := os.Executable()
 if err != nil { return "." }
 return filepath.Dir(exe)
}
func runtimeRoot() string { return filepath.Join(appDir(), "runtime") }

func checkTaskOpen() error {
 processMu.Lock(); defer processMu.Unlock()
 if appClosing { return errors.New("任务已停止") }
 return nil
}

// Start and register under the same lock: closing cannot miss a just-started child.
func startManaged(cmd *exec.Cmd) error {
 processMu.Lock(); defer processMu.Unlock()
 if appClosing { return errors.New("软件正在关闭") }
 if currentCmd != nil { return errors.New("上一个本地任务尚未退出") }
 cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow:true}
 if err := cmd.Start(); err != nil { return err }
 currentCmd = cmd
 return nil
}
func waitManaged(cmd *exec.Cmd) error {
 err := cmd.Wait()
 processMu.Lock()
 if currentCmd == cmd { currentCmd = nil }
 processMu.Unlock()
 return err
}
func runHidden(name string, args ...string) error {
 cmd := exec.Command(name, args...)
 if err := startManaged(cmd); err != nil { return err }
 return waitManaged(cmd)
}
func execHidden(name string, args ...string) {
 // Only used for opening the user's result folder, never recognition.
 cmd := exec.Command(name, args...)
 cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow:true}
 if err := cmd.Start(); err == nil { go cmd.Wait() }
}
func cancelRunningProcess() {
 processMu.Lock(); defer processMu.Unlock()
 appClosing = true
 if currentCmd != nil && currentCmd.Process != nil { _ = currentCmd.Process.Kill() }
}

func componentPath(root, name string) (string,error) {
 p, err := filepath.Abs(filepath.Join(root,name))
 if err != nil { return "",err }
 st,err:=os.Stat(p)
 if err!=nil || st.IsDir() || st.Size()==0 { return "",fmt.Errorf("本地组件不完整：%s，请重新完整解压软件包",name) }
 return p,nil
}
func findRecursive(root,name string)(string,error){
 var found string
 _ = filepath.Walk(root,func(path string,info os.FileInfo,err error) error{
  if err!=nil||info==nil{return nil}
  if !info.IsDir()&&strings.EqualFold(info.Name(),name){ found=path; return filepath.SkipAll }
  return nil
 })
 if found=="" { return "",fmt.Errorf("本地组件缺失：%s",name) }
 return filepath.Abs(found)
}

// Copies only user audio/model DATA. Never releases or downloads executable code.
func copyTaskFile(src,dst string) error {
 in,err:=os.Open(src); if err!=nil{return err}; defer in.Close()
 out,err:=os.OpenFile(dst,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600); if err!=nil{return err}
 good:=false
 defer func(){ out.Close(); if !good { _=os.Remove(dst) } }()
 buf:=make([]byte,1024*1024)
 for {
  if err=checkTaskOpen();err!=nil{return err}
  n,re:=in.Read(buf)
  if n>0 { if _,err=out.Write(buf[:n]);err!=nil{return err} }
  if re==io.EOF{break};if re!=nil{return re}
 }
 if err=out.Close();err!=nil{return err};good=true;return nil
}

// Keep stderr bounded. Do not collect stdout (recognized speech) or the script.
type diagnosticTail struct { mu sync.Mutex; text string }
func (d *diagnosticTail) Write(b []byte)(int,error){
 d.mu.Lock(); defer d.mu.Unlock()
 d.text += string(b)
 if len(d.text)>32768 { d.text=d.text[len(d.text)-32768:] }
 return len(b),nil
}
func (d *diagnosticTail) String() string { d.mu.Lock();defer d.mu.Unlock();return d.text }
func taskFailure(stage string,err error,tail string) error {
 code:="启动失败"
 var ee *exec.ExitError
 if errors.As(err,&ee){code=fmt.Sprintf("0x%08X",uint32(ee.ExitCode()))}
 // Log contains only stage, exit code and engine stderr. User files stay local.
 header:=fmt.Sprintf("906 Subtitle Tool %s\r\n%s\r\nStage: %s\r\nExit: %s\r\nError: %v\r\n\r\n",appVersion,time.Now().Format(time.RFC3339),stage,code,err)
 var saved string
 roots:=[]string{appDir(),filepath.Join(os.Getenv("LOCALAPPDATA"),"906SubtitleTool","logs")}
 for _,root:=range roots{
  if root==""{continue};if os.MkdirAll(root,0700)!=nil{continue}
  p:=filepath.Join(root,"906_subtitle_error.log")
  if os.WriteFile(p,[]byte(header+tail),0600)==nil{saved=p;break}
 }
 if saved!=""{return fmt.Errorf("%s失败（%s）。详情已保存：%s",stage,code,saved)}
 return fmt.Errorf("%s失败（%s）：%v",stage,code,err)
}
