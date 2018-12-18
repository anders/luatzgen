package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

var tzinfoVersion = "2018g"
var outfile = "zonedata.zip"

type tzhead struct {
	Magic           [4]byte
	Version         uint8
	_               [15]byte
	TransIsGMTCount uint32
	TransIsStdCount uint32
	LeapCount       uint32
	TimeCount       uint32
	TypeCount       uint32
	CharCount       uint32
}

type tzentry struct {
	Offset    uint32
	IsDST     bool
	AbbrIndex uint8
}

type tzleap struct {
	A uint32
	B uint32
}

type tz struct {
	Header     tzhead
	Times      []uint32
	Types      []uint8
	Offsets    []tzentry
	ZoneAbbrs  []byte
	Leaps      []tzleap
	IsStandard []bool
	IsUT       []bool
}

func toLua(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	tz := tz{}

	binary.Read(f, binary.BigEndian, &tz.Header)

	m := tz.Header.Magic
	if m[0] != 'T' || m[1] != 'Z' || m[2] != 'i' || m[3] != 'f' {
		return []byte{}, fmt.Errorf("magic != TZif")
	}

	for i := uint32(0); i < tz.Header.TimeCount; i++ {
		var time uint32
		binary.Read(f, binary.BigEndian, &time)
		tz.Times = append(tz.Times, time)
	}

	for i := uint32(0); i < tz.Header.TimeCount; i++ {
		var t uint8
		binary.Read(f, binary.BigEndian, &t)
		tz.Types = append(tz.Types, t)
	}

	for i := uint32(0); i < tz.Header.TypeCount; i++ {
		var o tzentry
		binary.Read(f, binary.BigEndian, &o)
		tz.Offsets = append(tz.Offsets, o)
	}

	tz.ZoneAbbrs = make([]byte, tz.Header.CharCount)
	f.Read(tz.ZoneAbbrs)

	for i := uint32(0); i < tz.Header.LeapCount; i++ {
		var l tzleap
		binary.Read(f, binary.BigEndian, &l)
		tz.Leaps = append(tz.Leaps, l)
	}

	for i := uint32(0); i < tz.Header.TransIsStdCount; i++ {
		var std uint8
		binary.Read(f, binary.BigEndian, &std)
		tz.IsStandard = append(tz.IsStandard, std == 1)
	}

	for i := uint32(0); i < tz.Header.TransIsGMTCount; i++ {
		var ut uint8
		binary.Read(f, binary.BigEndian, &ut)
		tz.IsUT = append(tz.IsUT, ut == 1)
	}

	name := stripPath(path)

	return tz.ToLua(name), nil
}

func stripPath(p string) string {
	tmp := strings.Split(p, "/")
	found := -1
	for i := 0; i < len(tmp); i++ {
		if tmp[i] == "zoneinfo" {
			found = i
		}
	}
	name := "???"
	if found > 0 {
		name = strings.Join(tmp[found+1:], "/")
	}
	return name
}

func main() {
	buf, err := os.Create(outfile)
	if err != nil {
		log.Fatal(err)
	}
	w := zip.NewWriter(buf)

	// TODO: don't hard code the path
	filepath.Walk("/usr/share/zoneinfo", func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			b, err := toLua(path)
			if err == nil {
				w, _ := w.Create("zonedata/" + stripPath(path))
				w.Write(b)
			}
		}
		return nil
	})
	if err := w.Close(); err != nil {
		log.Fatal(err)
	}
}

func readZeroString(data []byte, offset int) string {
	buf := []byte{}
	for {
		c := data[offset]
		if c == 0 {
			break
		}
		buf = append(buf, c)
		offset++
	}
	return string(buf)
}

func (tz tz) ToLua(zoneName string) []byte {
	buf := new(bytes.Buffer)

	fmt.Fprintf(buf, "{ version = \"%s\", zone = \"%s\",\n", tzinfoVersion, zoneName)

	for i := uint32(0); i < tz.Header.TimeCount; i++ {
		tt := tz.Times[i]
		o := tz.Offsets[tz.Types[i]]

		abbr := readZeroString(tz.ZoneAbbrs, int(o.AbbrIndex))
		fmt.Fprintf(buf, "  {ts=%d, dst=%t, name=\"%s\", ut_offset=%d},\n", tt, o.IsDST, abbr, o.Offset)
	}

	fmt.Fprintf(buf, "}\n")

	return buf.Bytes()
}
