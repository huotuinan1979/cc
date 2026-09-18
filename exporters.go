//go:build windows

package main

import (
	"encoding/csv"
		"fmt"
	"os"
	"image"
	"image/color"
	"image/png"
	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
	"path/filepath"
	"strconv"
	"strings"
)

func exportResult(subs []Subtitle, audio string, format int) (string, error) {
	dir := filepath.Dir(audio)
	base := strings.TrimSuffix(filepath.Base(audio), filepath.Ext(audio))
	switch format {
	case 0:
		p := filepath.Join(dir, base+"_字幕.fcpxml")
		return p, os.WriteFile(p, []byte(makeFCPXML(subs, base)), 0644)
	case 1:
		p := filepath.Join(dir, base+"_字幕.itt")
		return p, os.WriteFile(p, []byte(makeITT(subs)), 0644)
	case 2:
		p := filepath.Join(dir, base+"_字幕.srt")
		return p, os.WriteFile(p, []byte("\xef\xbb\xbf"+makeSRT(subs)), 0644)
	case 3:
		p := filepath.Join(dir, base+"_字幕.xml")
		return p, os.WriteFile(p, []byte(makeLegacyXML(subs, base)), 0644)
	case 4:
		p := filepath.Join(dir, base+"_透明字幕PNG")
		return p, exportPNG(subs, p)
	case 5:
		p := filepath.Join(dir, base+"_字幕时间表.csv")
		return p, exportCSV(subs, p)
	default:
		return "", fmt.Errorf("未知输出格式")
	}
}

func xmlEsc(s string) string {
	r := strings.NewReplacer("&","&amp;","<","&lt;",">","&gt;","\"","&quot;","'","&apos;")
	return r.Replace(s)
}

func srtTime(ms int64) string {
	if ms < 0 { ms = 0 }
	h := ms/3600000; ms%=3600000
	m := ms/60000; ms%=60000
	s := ms/1000; x := ms%1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h,m,s,x)
}

func dotTime(ms int64) string {
	if ms < 0 { ms = 0 }
	h := ms/3600000; ms%=3600000
	m := ms/60000; ms%=60000
	s := ms/1000; x := ms%1000
	return fmt.Sprintf("%02d:%02d:%02d.%03d", h,m,s,x)
}

func makeSRT(subs []Subtitle) string {
	var b strings.Builder
	for i,s := range subs {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, srtTime(s.StartMS), srtTime(s.EndMS), s.Text)
	}
	return b.String()
}

func makeITT(subs []Subtitle) string {
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<tt xmlns=\"http://www.w3.org/ns/ttml\" xmlns:tts=\"http://www.w3.org/ns/ttml#styling\" xml:lang=\"zh-CN\">\n")
	b.WriteString("  <head><styling><style xml:id=\"s1\" tts:textAlign=\"center\"/></styling><layout><region xml:id=\"bottom\" tts:origin=\"10% 80%\" tts:extent=\"80% 18%\"/></layout></head>\n")
	b.WriteString("  <body region=\"bottom\"><div>\n")
	for _,s := range subs {
		fmt.Fprintf(&b, "    <p begin=\"%s\" end=\"%s\" style=\"s1\">%s</p>\n", dotTime(s.StartMS), dotTime(s.EndMS), xmlEsc(s.Text))
	}
	b.WriteString("  </div></body>\n</tt>\n")
	return b.String()
}

func makeFCPXML(subs []Subtitle, name string) string {
	dur := int64(1000)
	if len(subs)>0 { dur = subs[len(subs)-1].EndMS+500 }
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE fcpxml>\n")
	b.WriteString("<fcpxml version=\"1.10\">\n  <resources>\n")
	b.WriteString("    <format id=\"r1\" frameDuration=\"1/25s\" width=\"1920\" height=\"1080\" colorSpace=\"1-1-1 (Rec. 709)\"/>\n")
	b.WriteString("  </resources>\n  <library><event name=\"玖零六字幕\"><project name=\""+xmlEsc(name)+" 字幕\">\n")
	fmt.Fprintf(&b, "    <sequence format=\"r1\" duration=\"%d/1000s\" tcStart=\"0/1s\" tcFormat=\"NDF\" audioLayout=\"stereo\" audioRate=\"48k\">\n", dur)
	b.WriteString("      <spine>\n")
	fmt.Fprintf(&b, "        <gap name=\"字幕时间线\" offset=\"0/1s\" duration=\"%d/1000s\">\n", dur)
	for i,s := range subs {
		d := s.EndMS-s.StartMS
		if d < 1 { d = 1 }
		fmt.Fprintf(&b, "          <caption name=\"%s\" lane=\"1\" offset=\"%d/1000s\" start=\"%d/1000s\" duration=\"%d/1000s\" role=\"iTT?captionFormat=ITT.zh\">\n", xmlEsc(shortName(s.Text)), s.StartMS,s.StartMS,d)
		fmt.Fprintf(&b, "            <text><text-style ref=\"cts%d\">%s</text-style></text>\n", i+1, xmlEsc(s.Text))
		fmt.Fprintf(&b, "            <text-style-def id=\"cts%d\"><text-style font=\"PingFang SC\" fontSize=\"60\" fontColor=\"1 1 1 1\" bold=\"1\" alignment=\"center\"/></text-style-def>\n", i+1)
		b.WriteString("          </caption>\n")
	}
	b.WriteString("        </gap>\n      </spine>\n    </sequence>\n  </project></event></library>\n</fcpxml>\n")
	return b.String()
}

func shortName(s string) string {
	r := []rune(strings.TrimSpace(s))
	if len(r)>14 { r=r[:14] }
	return string(r)
}

func makeLegacyXML(subs []Subtitle, name string) string {
	last := int64(1000)
	if len(subs)>0 { last=subs[len(subs)-1].EndMS }
	totalFrames := int(last*25/1000)+25
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE xmeml>\n<xmeml version=\"5\">\n")
	b.WriteString("<sequence><name>"+xmlEsc(name)+" 字幕</name><duration>"+strconv.Itoa(totalFrames)+"</duration><rate><timebase>25</timebase><ntsc>FALSE</ntsc></rate><media><video><track>\n")
	for i,s := range subs {
		inF:=int(s.StartMS*25/1000); outF:=int(s.EndMS*25/1000); if outF<=inF {outF=inF+1}
		fmt.Fprintf(&b, "<generatoritem id=\"subtitle-%d\"><name>%s</name><start>%d</start><end>%d</end><in>0</in><out>%d</out><rate><timebase>25</timebase><ntsc>FALSE</ntsc></rate><effect><name>Text</name><effectid>Text</effectid><effectcategory>Text</effectcategory><effecttype>generator</effecttype><mediatype>video</mediatype><parameter><parameterid>str</parameterid><name>Text</name><value>%s</value></parameter></effect></generatoritem>\n", i+1, xmlEsc(shortName(s.Text)), inF,outF,outF-inF,xmlEsc(s.Text))
	}
	b.WriteString("</track></video></media></sequence>\n</xmeml>\n")
	return b.String()
}

func exportCSV(subs []Subtitle, path string) error {
	f,err:=os.Create(path); if err!=nil{return err}; defer f.Close()
	_,_ = f.Write([]byte{0xef,0xbb,0xbf})
	w:=csv.NewWriter(f); defer w.Flush()
	_ = w.Write([]string{"序号","开始时间","结束时间","字幕"})
	for i,s:=range subs { _=w.Write([]string{strconv.Itoa(i+1),dotTime(s.StartMS),dotTime(s.EndMS),s.Text}) }
	return w.Error()
}

func exportPNG(subs []Subtitle, dir string) error {
	_ = os.RemoveAll(dir)
	if err:=os.MkdirAll(dir,0755);err!=nil{return err}

	fontPath:=`C:\\Windows\\Fonts\\msyh.ttc`
	data,err:=os.ReadFile(fontPath)
	if err!=nil{
		fontPath=`C:\\Windows\\Fonts\\simhei.ttf`
		data,err=os.ReadFile(fontPath)
		if err!=nil{return fmt.Errorf("无法读取系统中文字体")}
	}

	var face font.Face
	if strings.HasSuffix(strings.ToLower(fontPath),".ttc"){
		col,err:=opentype.ParseCollection(data)
		if err!=nil{return err}
		f,err:=col.Font(0)
		if err!=nil{return err}
		face,err=opentype.NewFace(f,&opentype.FaceOptions{Size:54,DPI:96,Hinting:font.HintingFull})
		if err!=nil{return err}
	}else{
		f,err:=opentype.Parse(data)
		if err!=nil{return err}
		face,err=opentype.NewFace(f,&opentype.FaceOptions{Size:54,DPI:96,Hinting:font.HintingFull})
		if err!=nil{return err}
	}
	defer face.Close()

	for i,sub:=range subs{
		img:=image.NewNRGBA(image.Rect(0,0,1920,1080))
		d:=&font.Drawer{Dst:img,Src:image.NewUniform(color.NRGBA{255,255,255,255}),Face:face}
		text:=sub.Text
		advance:=d.MeasureString(text)
		x:=(fixed.I(1920)-advance)/2
		y:=fixed.I(930)
		d.Dot=fixed.Point26_6{X:x,Y:y}
		d.DrawString(text)

		p:=filepath.Join(dir,fmt.Sprintf("%04d.png",i+1))
		f,err:=os.Create(p)
		if err!=nil{return err}
		if err=png.Encode(f,img);err!=nil{f.Close();return err}
		f.Close()
		updateStatus(88+(i+1)*10/maxInt(1,len(subs)),"正在生成透明字幕图片…",false,"")
	}
	return nil
}

func maxInt(a,b int)int{if a>b{return a};return b}
