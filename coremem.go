package main

import (
	"bufio"
	"bytes"
	"crypto/md5"
	// "flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	// "os/exec"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

const procstr = "/proc"
const PAGESIZE = 4096

var verbose int

func procPath(pid string, name ...string) string {
	return path.Join(append([]string{procstr, pid},
		name...)...)
}

func procRead(pid string, name ...string) ([]byte, error) {
	r, err := os.Open(procPath(pid, name...))
	if err != nil {
		return nil, err
	}
	defer r.Close()

	return ioutil.ReadAll(r)
}

func procReadLine(pid string, name ...string) string {
	r, err := os.Open(procPath(pid, name...))
	if err != nil {
		return ""
	}
	defer r.Close()

	buf := bufio.NewScanner(r)
	buf.Scan()
	return buf.Text()
}

func listpids() <-chan int {
	pids := make(chan int, 100)

	go func() {
		defer close(pids)

		proc, _ := os.Open(procstr)
		defer proc.Close()

		for {
			fis, err := proc.Readdir(100)
			if err != nil {
				if err != io.EOF {
					fmt.Fprintf(os.Stderr, "what's wrong %q, %#v\n", err, err)
				}
				return
			}
			for _, fi := range fis {
				if !fi.IsDir() {
					continue
				}
				pid, err := strconv.Atoi(fi.Name())
				if err != nil {
					// non digits, not a pid
					continue
				}
				pids <- pid
			}
		}
	}()
	return pids
}

type meminfo struct {
	private int
	shared  int
	rss     int
	pss     int
	count   int
	cmd     string
	mem_id  []byte
}

func (mi *meminfo) cmdStr() string {
	if mi.count > 1 {
		return fmt.Sprintf("%s (%d)", mi.cmd, mi.count)
	} else {
		return mi.cmd
	}
}

type memis []*meminfo

func (s memis) Len() int      { return len(s) }
func (s memis) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

type ByPss struct{ memis }

func (s ByPss) Less(i, j int) bool { return s.memis[i].pss < s.memis[j].pss }

func human(x int) string {
	var powers = []string{"Ki", "Mi", "Gi", "Ti"}
	pidx := 0
	num := float32(x)
	for num >= 1000 {
		num /= 1024.0
		pidx++
	}
	return fmt.Sprintf("%.1f %sB", num, powers[pidx])
}

func dispatch(ch <-chan int) <-chan *meminfo {
	out := make(chan *meminfo, 100)
	go func() {
		var wg sync.WaitGroup
		for runtime.NumGoroutine() <= 10*runtime.NumCPU() {
			wg.Add(1)
			go process(ch, out, func() { wg.Done() })
		}
		wg.Wait()
		close(out)
	}()
	return out
}

func process(ch <-chan int, out chan<- *meminfo, done func()) {
	defer done()
	for pid := range ch {
		readMeminfo(pid, out)
	}
}

func readMeminfo(pid int, out chan<- *meminfo) {
	pidname := fmt.Sprint(pid)
	// fmt.Printf("got %q\n", pidname)
	cmdline, _ := procRead(pidname, "cmdline")

	// kernel threads have empty cmdline
	if len(cmdline) == 0 {
		return
	}

	cmdargs := bytes.Split(cmdline, []byte{0})
	cmdl := string(cmdargs[0])
	exepath, err := os.Readlink(procPath(pidname, "exe"))
	if err != nil {
		// permission error?
		return
	}
	if strings.HasSuffix(exepath, "(deleted)") {
		exepath = exepath[:len(exepath)-10]
		if err := syscall.Access(exepath, syscall.F_OK); err == nil {
			exepath += " [updated]"
		} else if err := syscall.Access(cmdl, syscall.F_OK); err == nil {
			exepath = cmdl + " [updated]"
		} else {
			exepath += " [deleted]"
		}
	}
	exe := path.Base(exepath)

	// the first line of "status" is like, with tab between, not spaces
	// Name:   bash
	cmd := procReadLine(pidname, "status")[6:]
	if strings.HasPrefix(exe, cmd) {
		cmd = exe
	}

	var rss, shared, private, pss int
	// "statm" shows unit in pages
	rss, _ = strconv.Atoi(strings.Fields(procReadLine(pidname, "statm"))[1])
	rss *= PAGESIZE / 1024

	smapsPath := procPath(pidname, "smaps")
	// execMd5 := exec.Command("md5sum", smapsPath)
	// out, _ := execMd5.CombinedOutput()
	// fmt.Printf("%q", out)
	smapsF, err := os.Open(smapsPath)
	if err != nil {
		// too old kernel, no smaps?
		return
	}
	defer smapsF.Close()

	dige := md5.New()
	buf := bufio.NewReader(smapsF)
	for line, err := buf.ReadString('\n'); err == nil; line, err = buf.ReadString('\n') {
		io.WriteString(dige, line)
		if strings.HasPrefix(line, "Shared") {
			num, _ := strconv.Atoi(strings.Fields(line)[1])
			shared += num
		} else if strings.HasPrefix(line, "Private") {
			num, _ := strconv.Atoi(strings.Fields(line)[1])
			private += num
		} else if strings.HasPrefix(line, "Pss") {
			num, _ := strconv.Atoi(strings.Fields(line)[1])
			pss += num
			// total += num
		}
	}
	mem_id := dige.Sum(nil)

	mi := &meminfo{
		count:   1,
		private: private,
		shared:  pss - private,
		rss:     rss,
		pss:     pss,
		mem_id:  mem_id,
		cmd:     cmd}

	if verbose >= 1 {
		fmt.Fprintf(os.Stderr, "proc %v\t %q (%q %q) %v %v %v %v %x\n",
			pid, exe, cmd, cmdl,
			rss, shared, private, pss, mem_id)
	}

	out <- mi
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	if len(os.Args) > 1 && os.Args[1] == "-v" {
		verbose++
	}

	fmt.Print("  Private  (  Shared)    RAM (PSS)\tProgram\n\n")

	var (
		mems  = make(map[string]*meminfo, 100)
		mis   = make([]*meminfo, 0, 100)
		total = 0
	)

	for mi := range dispatch(listpids()) {
		if mie, ok := mems[mi.cmd]; ok {
			mie.count++
			mie.private += mi.private
			mie.rss += mi.rss
			mie.pss += mi.pss
			mie.shared += mi.shared
		} else {
			mis = append(mis, mi)
			mems[mi.cmd] = mi
		}
		total += mi.pss
	}

	sort.Sort(ByPss{mis})
	for _, mi := range mis {
		fmt.Printf("%9s  (%9s)  %9s\t%s\n",
			human(mi.private),
			human(mi.shared),
			human(mi.pss),
			mi.cmdStr())
	}

	fmt.Printf("%s\n%33s\n%s\n",
		strings.Repeat("-", 33),
		fmt.Sprintf("Pss total: %s", human(total)),
		strings.Repeat("=", 33))
}
