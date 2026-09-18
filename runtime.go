//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

var (
	processMu sync.Mutex
	currentCmd *exec.Cmd
)

func appDir() string {
	exe, err := os.Executable()
	if err != nil { return "." }
	return filepath.Dir(exe)
}

func runtimeRoot() string {
	return filepath.Join(appDir(), "runtime")
}

func runHidden(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow:true}
	processMu.Lock()
	currentCmd = cmd
	processMu.Unlock()
	err := cmd.Run()
	processMu.Lock()
	if currentCmd == cmd { currentCmd = nil }
	processMu.Unlock()
	return err
}

func execHidden(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow:true}
	_ = cmd.Start()
}

func cancelRunningProcess() {
	processMu.Lock()
	cmd := currentCmd
	processMu.Unlock()
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func componentPath(root, name string) (string,error) {
	p:=filepath.Join(root,name)
	if _,err:=os.Stat(p);err!=nil {
		return "",fmt.Errorf("本地组件缺失：%s",name)
	}
	return p,nil
}

func findRecursive(root,name string)(string,error){
	var found string
	_ = filepath.Walk(root,func(path string,info os.FileInfo,err error) error{
		if err!=nil||info==nil{return nil}
		if !info.IsDir()&&strings.EqualFold(info.Name(),name){
			found=path
			return filepath.SkipAll
		}
		return nil
	})
	if found=="" { return "",fmt.Errorf("本地组件缺失：%s",name) }
	return found,nil
}
