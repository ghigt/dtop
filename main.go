package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	gnc "code.google.com/p/goncurses"
	"github.com/docker/docker/pkg/units"
	"github.com/fsouza/go-dockerclient"
)

var (
	fHost  = flag.String("host", "DOCKER_HOST", "docker host location")
	fCerts = flag.String("certs", "DOCKER_CERT_PATH", "docker certs location")
)

const (
	NB_COLUMNS  = 8
	HEADER_SIZE = 2
)

const (
	COLOR_CONTAINER = iota + 2
	COLOR_SELECTION
	COLOR_HELP
	COLOR_RUNNING
	COLOR_HIGHLIGHT
)

type Status int // What we are doing

const (
	STATUS_POT     = iota // Currently displaying containers
	STATUS_HELP           // Currently displaying help
	STATUS_CONFIRM        // Currently waiting for confirmation
	STATUS_INFO           // Currently displaying info
)

type Sort int

const (
	SORT_NAME = iota
	SORT_IMAGE
	SORT_ID
	SORT_COMMAND
	SORT_UPTIME
	SORT_STATUS
	SORT_CPU
	SORT_RAM
)

// CommonLine contains information common to each printed line
type CommonLine struct {
	Id      string // PID or docker hash
	Command string // command name or CMD
	Uptime  string // container uptime or process uptime
	Status  string // container status or process state
	CPU     string // % of CPU used (if container, sum of % of processes)
	RAM     string // RAM used (if container, sum of RAM of processes)
}

// ContainerLine contains information specific to a docker container line
type ContainerLine struct {
	Name       string // docker container name
	Image      string // docker container image
	CommonLine        // same props as processes
}

func PrettyColumn(in string, expected_len int, prefix string, suffix string) string {
	i := len(in) + len(prefix) + len(suffix)
	if i < expected_len {
		return prefix + in + strings.Repeat(" ", expected_len-i) + suffix
	}
	if i > expected_len {
		j := expected_len - len(prefix) - len(suffix)
		return prefix + in[0:j] + suffix
	}

	return prefix + in + suffix
}

func (c *ContainerLine) Format(column_width int) string {
	return PrettyColumn(c.Name, column_width, " ", " ") +
		PrettyColumn(c.Image, column_width, " ", " ") +
		PrettyColumn(c.Id, column_width, " ", " ") +
		PrettyColumn(c.Command, column_width, " ", " ") +
		PrettyColumn(c.Uptime, column_width, " ", " ") +
		PrettyColumn(c.Status, column_width, " ", " ") +
		PrettyColumn(c.CPU, column_width, " ", " ") +
		PrettyColumn(c.RAM, column_width, " ", " ")
}

// ProcessLine contains information about a process
type ProcessLine ContainerLine

func (c *ProcessLine) Format(column_width int) string {
	return PrettyColumn("", column_width, " ", " ") +
		PrettyColumn("", column_width, " ", " ") +
		PrettyColumn(c.Id, column_width, " |- ", " ") +
		PrettyColumn(c.Command, column_width, " ", " ") +
		PrettyColumn(c.Uptime, column_width, " ", " ") +
		PrettyColumn(c.Status, column_width, " ", " ") +
		PrettyColumn(c.CPU, column_width, " ", " ") +
		PrettyColumn(c.RAM, column_width, " ", " ")
}

type Container struct {
	container     ContainerLine // information about the container
	processes     []ProcessLine // information about the processes
	isSelected    bool          // container selection
	showProcesses bool          // whether or not to show processes
}

type PrintedLine struct {
	line        string // the line
	isContainer bool   // is this line a container?
	isProcess   bool   // is this line a process?
	isActive    bool   // is this line selected?
	isRunning   bool   // is the container running
}

type Pot struct {
	c                   *docker.Client // Used to talk to the daemon
	status              Status         // Current status
	snapshot            []Container    // Current containers/processes state
	win                 *gnc.Window    // goncurse Window
	showGlobalProcesses bool           // whether or not to show processes
	reverse             bool           // Reverse sort
	sort                Sort           // Current sort
	currentInfo         Container
}

var (
	active = 0
	scroll = 0
)

// Returns the running processes for the current Container
func (pot *Pot) GetProcesses(cid string) []ProcessLine {
	res := make([]ProcessLine, 0, 10)
	//val := url.Values{}
	//val.Set("ps_args", "-o pid,etime,%cpu,%mem,cmd")
	//stream, _, err := pot.c.call("GET", "/containers/"+cid+"/top?"+val.Encode(), nil, false)
	//if err != nil {
	//	return res
	//}
	//var procs engine.Env
	//if err := procs.Decode(stream); err != nil {
	//	return res
	//}
	//processes := [][]string{}
	//if err := procs.GetJson("Processes", &processes); err != nil {
	//	return res
	//}

	topr, e := pot.c.TopContainer(cid, url.QueryEscape("o pid,etime,%cpu,%mem,cmd"))
	if e != nil {

		// /!\ NEED TO SEE IF ERROR APPEND !

		return res
	}

	for _, proc := range topr.Processes {

		var p ProcessLine

		p.Id = proc[0]
		p.Uptime = proc[1]
		p.CPU = proc[2]
		p.RAM = proc[3]
		p.Command = proc[4]

		res = append(res, p)
	}

	return res
}

// Returns the list of running containers as well as internal processes
func (pot *Pot) Snapshot() []Container {
	res := make([]Container, 0, 16)

	//v := url.Values{}
	//v.Set("all", "1")
	//body, _, err := readBody(pot.c.call("GET", "/containers/json?"+v.Encode(), nil, false))
	//if err != nil {
	//	return res
	//}
	//outs := engine.NewTable("Created", 0)
	//if _, err = outs.ReadListFrom(body); err != nil {
	//	return res
	//}

	cnts, e := pot.c.ListContainers(docker.ListContainersOptions{All: true})
	if e != nil {

		// /!\ NEED TO SEE IF ERROR APPEND

		return res
	}
	for _, cnt := range cnts {
		var c Container

		c.showProcesses = false
		c.container.Id = cnt.ID
		c.container.Command = cnt.Command
		c.container.Image = cnt.Image
		c.container.Name = cnt.Names[0]
		c.container.Uptime = units.HumanDuration(time.Now().UTC().Sub(time.Unix(cnt.Created, 0)))
		c.container.Status = cnt.Status

		if strings.HasPrefix(c.container.Status, "Up") {
			c.processes = pot.GetProcesses(c.container.Id)
		}

		total_cpu := 0.0
		total_ram := 0.0
		for _, p := range c.processes {
			cpu, err := strconv.ParseFloat(p.CPU, 32)
			if err == nil {
				total_cpu = total_cpu + cpu
			}
			ram, err := strconv.ParseFloat(p.RAM, 32)
			if err == nil {
				total_ram = total_ram + ram
			}
		}
		c.container.CPU = fmt.Sprintf("%.1f", total_cpu)
		c.container.RAM = fmt.Sprintf("%.1f", total_ram)

		for _, cn := range pot.snapshot {
			if cn.container.Id == c.container.Id {
				c.isSelected = cn.isSelected
				c.showProcesses = cn.showProcesses
				break
			}
		}

		res = append(res, c)
	}

	return res
}

func (pot *Pot) PrintActive(l PrintedLine, lc int, i int) {
	if i < scroll || i >= scroll+lc {
		return
	}
	if active == i {
		pot.win.AttrOn(gnc.A_REVERSE)
		pot.win.Println(l.line)
		pot.win.AttrOff(gnc.A_REVERSE)
	} else {
		if l.isActive {
			pot.win.ColorOn(COLOR_SELECTION)
			pot.win.Println(l.line)
			pot.win.ColorOff(COLOR_SELECTION)
		} else if l.isContainer {
			if l.isRunning {
				pot.win.ColorOn(COLOR_RUNNING)
				pot.win.Println(l.line)
				pot.win.ColorOff(COLOR_RUNNING)
			} else {
				pot.win.ColorOn(COLOR_CONTAINER)
				pot.win.Println(l.line)
				pot.win.ColorOff(COLOR_CONTAINER)
			}
		} else {
			pot.win.Println(l.line)
		}
	}
}

type SortableContainers struct {
	containers []Container
	sort       Sort // property to sort on
	reverse    bool // sort is reversed
}

func (a SortableContainers) Len() int { return len(a.containers) }
func (a SortableContainers) Swap(i, j int) {
	a.containers[i], a.containers[j] = a.containers[j], a.containers[i]
}
func (a SortableContainers) Less(i, j int) bool {
	var less bool

	switch a.sort {
	case SORT_NAME:
		if a.containers[i].container.Name == a.containers[j].container.Name {
			less = a.containers[i].container.Id < a.containers[j].container.Id
		} else {
			less = a.containers[i].container.Name < a.containers[j].container.Name
		}
	case SORT_IMAGE:
		if a.containers[i].container.Image == a.containers[j].container.Image {
			less = a.containers[i].container.Id < a.containers[j].container.Id
		} else {
			less = a.containers[i].container.Image < a.containers[j].container.Image
		}
	case SORT_ID:
		// always unique
		less = a.containers[i].container.Id < a.containers[j].container.Id
	case SORT_COMMAND:
		if a.containers[i].container.Command == a.containers[j].container.Command {
			less = a.containers[i].container.Id < a.containers[j].container.Id
		} else {
			less = a.containers[i].container.Command < a.containers[j].container.Command
		}
	case SORT_UPTIME:
		if a.containers[i].container.Uptime == a.containers[j].container.Uptime {
			less = a.containers[i].container.Id < a.containers[j].container.Id
		} else {
			less = a.containers[i].container.Uptime < a.containers[j].container.Uptime
		}
	case SORT_CPU:
		if a.containers[i].container.CPU == a.containers[j].container.CPU {
			less = a.containers[i].container.Id < a.containers[j].container.Id
		} else {
			cpu_a, _ := strconv.ParseFloat(a.containers[i].container.CPU, 32)
			cpu_b, _ := strconv.ParseFloat(a.containers[j].container.CPU, 32)
			less = cpu_a > cpu_b
		}
	case SORT_RAM:
		if a.containers[i].container.RAM == a.containers[j].container.RAM {
			less = a.containers[i].container.Id < a.containers[j].container.Id
		} else {
			ram_a, _ := strconv.ParseFloat(a.containers[i].container.RAM, 32)
			ram_b, _ := strconv.ParseFloat(a.containers[j].container.RAM, 32)
			less = ram_a > ram_b
		}
	case SORT_STATUS:
		if a.containers[i].container.Status == a.containers[j].container.Status {
			less = a.containers[i].container.Id < a.containers[j].container.Id
		} else {
			less = a.containers[i].container.Status < a.containers[j].container.Status
		}
	default:

		less = i < j
	}

	if a.reverse {
		less = !less
	}

	return less
}

func (pot *Pot) UpdatePot(lc int, wc int) {
	ss := make([]PrintedLine, 0, 42)

	sort.Sort(SortableContainers{pot.snapshot, pot.sort, pot.reverse})

	for _, cnt := range pot.snapshot {
		p := PrintedLine{
			line:        cnt.container.Format(wc),
			isContainer: true,
			isProcess:   false,
			isActive:    cnt.isSelected,
		}
		if len(cnt.container.Status) > 2 && cnt.container.Status[0:2] == "Up" {
			p.isRunning = true
		}
		ss = append(ss, p)
		if pot.showGlobalProcesses || cnt.showProcesses {
			for _, proc := range cnt.processes {
				ss = append(ss, PrintedLine{proc.Format(wc), false, true, false, false})
			}
		}
	}
	if active < 0 {
		active = 0
	} else if active >= len(ss) {
		active = len(ss) - 1
	}
	if active >= scroll+lc {
		scroll = active - lc + 1
	}
	if active < scroll {
		scroll = active
	}
	for i, s := range ss {
		pot.PrintActive(s, lc, i)
	}
}

func (pot *Pot) colorColumn(s string, sort Sort) {
	if pot.sort == sort {
		pot.win.AttrOn(gnc.A_REVERSE)
		pot.win.ColorOn(COLOR_HIGHLIGHT)
		pot.win.Printf("%s", s)
		pot.win.ColorOff(COLOR_HIGHLIGHT)
		pot.win.AttrOff(gnc.A_REVERSE)
		return
	}
	pot.win.AttrOn(gnc.A_REVERSE)
	pot.win.Printf("%s", s)
	pot.win.AttrOff(gnc.A_REVERSE)
}

func (pot *Pot) PrintHeader(wc int) {
	o, _ := exec.Command("uptime").Output()
	pot.win.Printf("%s", o)

	pot.colorColumn(PrettyColumn("Name", wc, " ", " "), SORT_NAME)
	pot.colorColumn(PrettyColumn("Image", wc, " ", " "), SORT_IMAGE)
	pot.colorColumn(PrettyColumn("Id", wc, " ", " "), SORT_ID)
	pot.colorColumn(PrettyColumn("Command", wc, " ", " "), SORT_COMMAND)
	pot.colorColumn(PrettyColumn("Uptime", wc, " ", " "), SORT_UPTIME)
	pot.colorColumn(PrettyColumn("Status", wc, " ", " "), SORT_STATUS)
	pot.colorColumn(PrettyColumn("%CPU", wc, " ", " "), SORT_CPU)
	pot.colorColumn(PrettyColumn("%RAM", wc, " ", " "), SORT_RAM)
	pot.win.Println()
}

func (pot *Pot) PrintPot(wc int, lc int) {
	pot.PrintHeader(wc)
	pot.UpdatePot(lc, wc)
}

func (pot *Pot) PrintInfo(wc int) {
	var info = Page{
		Header: "\nInformation about container:\n",
		Info:   "",
		Body:   []ItemPair{},
		Footer: "\nPress 'i' to return.",
	}

	info.Body = append(info.Body, ItemPair{"Name", pot.currentInfo.container.Name})
	info.Body = append(info.Body, ItemPair{"Id", pot.currentInfo.container.Id})
	info.Body = append(info.Body, ItemPair{"Command", pot.currentInfo.container.Command})
	info.Body = append(info.Body, ItemPair{"Uptime", pot.currentInfo.container.Uptime})
	info.Body = append(info.Body, ItemPair{"Status", pot.currentInfo.container.Status})
	info.Body = append(info.Body, ItemPair{"%CPU", pot.currentInfo.container.CPU})
	info.Body = append(info.Body, ItemPair{"%RAM", pot.currentInfo.container.RAM})

	pot.PrintPage(info, wc)
}

func (pot *Pot) PrintPage(p Page, wc int) {
	pot.win.ColorOn(COLOR_SELECTION)
	pot.win.Printf("%s\n", p.Header)
	pot.win.ColorOff(COLOR_SELECTION)

	pot.win.Printf("%s", p.Info)

	for _, v := range p.Body {
		pot.win.ColorOn(COLOR_HELP)
		pot.win.Printf("%s", PrettyColumn(v.Com+":", 20, " ", " "))
		pot.win.ColorOff(COLOR_HELP)
		pot.win.Printf("%s", PrettyColumn(v.Def, 40, " ", " "))
		pot.win.Println()
	}

	pot.win.ColorOn(COLOR_CONTAINER)
	pot.win.Printf("%s\n", p.Footer)
	pot.win.ColorOff(COLOR_CONTAINER)
}

func (pot *Pot) PrintHelp(wc int) {
	pot.PrintPage(help, wc)
}

func (pot *Pot) GetContainerByPos(line_num int) int {
	i := 0

	for res, cnt := range pot.snapshot {
		if i == line_num {
			return res
		}
		if i > line_num {
			break
		}
		if pot.showGlobalProcesses || cnt.showProcesses {
			i += len(cnt.processes)
		}
		i++
	}

	return -1
}

func (pot *Pot) getSelectedContainers() []int {
	res := make([]int, 0, 5)
	for i, c := range pot.snapshot {
		if c.isSelected {
			res = append(res, i)
		}
	}
	if len(res) == 0 {
		c := pot.GetContainerByPos(active)
		if c != -1 {
			if !pot.snapshot[c].isSelected {
				res = append(res, c)
			}
		}
	}
	return res
}

func (pot *Pot) StopContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "stop", id).Run()
	}(id)
}

func (pot *Pot) StartContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "start", id).Run()
	}(id)
}

func (pot *Pot) RmContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "rm", id).Run()
	}(id)
}

func (pot *Pot) KillContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "kill", id).Run()
	}(id)
}

func (pot *Pot) PauseContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "pause", id).Run()
	}(id)
}

func (pot *Pot) UnpauseContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "unpause", id).Run()
	}(id)
}

func initClient() *docker.Client {
	var client *docker.Client
	var e error

	certs := os.Getenv(*fCerts)
	host := os.Getenv(*fHost)
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}

	if strings.HasPrefix(host, "unix") {
		client, e = docker.NewClient(host)
		if e != nil {
			log.Fatal(e)
		}
	} else {
		client, e = docker.NewTLSClient(host, certs+"/cert.pem", certs+"/key.pem", "")
		if e != nil {
			log.Fatal(e)
		}
	}

	return client
}

func (pot *Pot) Run() {
	var err error

	pot.win, err = gnc.Init()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer gnc.End()

	gnc.StartColor()
	gnc.InitPair(COLOR_CONTAINER, gnc.C_CYAN, gnc.C_BLACK)
	gnc.InitPair(COLOR_SELECTION, gnc.C_BLACK, gnc.C_YELLOW)
	gnc.InitPair(COLOR_HELP, gnc.C_YELLOW, gnc.C_BLACK)
	gnc.InitPair(COLOR_RUNNING, gnc.C_GREEN, gnc.C_BLACK)
	gnc.InitPair(COLOR_HIGHLIGHT, gnc.C_MAGENTA, gnc.C_BLACK)
	pot.win.Keypad(true)
	gnc.Echo(false)
	gnc.Cursor(0)

	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGWINCH)

	k := make(chan gnc.Key)
	t := time.Tick(time.Second)

	go func(scr *gnc.Window, c chan gnc.Key) {
		for {
			c <- scr.GetChar()
		}
	}(pot.win, k)

	pot.snapshot = pot.Snapshot()

	for {
		// Print screen
		my, mx := pot.win.MaxYX()
		lc := my - HEADER_SIZE
		wc := (mx - 1) / NB_COLUMNS
		pot.win.Erase()
		if mx < 40 || my < 5 {
			continue
		}

		switch pot.status {
		case STATUS_POT:
			pot.PrintPot(wc, lc)
			pot.win.Refresh()
		case STATUS_HELP:
			pot.PrintHelp(wc)
			pot.win.Refresh()
		case STATUS_INFO:
			pot.PrintInfo(wc)
			pot.win.Refresh()
		}

		// Handle Events
		select {
		case kk := <-k:
			if kk == 'q' {
				return
			}
			switch pot.status {
			case STATUS_POT:
				if kk == gnc.KEY_DOWN {
					active = active + 1
				}
				if kk == gnc.KEY_UP {
					active = active - 1
				}
				if kk == 'h' {
					pot.status = STATUS_HELP
				}
				if kk == 'A' {
					pot.showGlobalProcesses = !pot.showGlobalProcesses
				}
				if kk == 'k' {
					for _, c := range pot.getSelectedContainers() {
						pot.KillContainer(&pot.snapshot[c])
					}
				}
				if kk == 'u' {
					for i, _ := range pot.snapshot {
						pot.snapshot[i].isSelected = false
					}
				}
				if kk == 'a' {
					for _, c := range pot.getSelectedContainers() {
						pot.snapshot[c].showProcesses = !pot.snapshot[c].showProcesses
					}
				}
				if kk == ' ' {
					c := pot.GetContainerByPos(active)
					if c != -1 {
						pot.snapshot[c].isSelected = !pot.snapshot[c].isSelected
					}
					active = active + 1
				}
				if kk == 's' {
					for _, c := range pot.getSelectedContainers() {
						pot.StartContainer(&pot.snapshot[c])
					}
				}
				if kk == 'S' {
					for _, c := range pot.getSelectedContainers() {
						pot.StopContainer(&pot.snapshot[c])
					}
				}
				if kk == 'r' {
					for _, c := range pot.getSelectedContainers() {
						pot.RmContainer(&pot.snapshot[c])
					}
				}
				if kk == 'i' {
					c := pot.GetContainerByPos(active)
					if c != -1 {
						pot.currentInfo = pot.snapshot[c]
						pot.status = STATUS_INFO
					}
				}
				if kk == 'p' {
					for _, c := range pot.getSelectedContainers() {
						pot.PauseContainer(&pot.snapshot[c])
					}
				}
				if kk == 'P' {
					for _, c := range pot.getSelectedContainers() {
						pot.UnpauseContainer(&pot.snapshot[c])
					}
				}
			case STATUS_HELP:
				if kk == 'h' {
					pot.status = STATUS_POT
				}
			case STATUS_INFO:
				if kk == 'i' {
					pot.status = STATUS_POT
				}
			}
			if kk == '1' {
				pot.sort = SORT_NAME
			}
			if kk == '2' {
				pot.sort = SORT_IMAGE
			}
			if kk == '3' {
				pot.sort = SORT_ID
			}
			if kk == '4' {
				pot.sort = SORT_COMMAND
			}
			if kk == '5' {
				pot.sort = SORT_UPTIME
			}
			if kk == '6' {
				pot.sort = SORT_STATUS
			}
			if kk == '7' {
				pot.sort = SORT_CPU
			}
			if kk == '8' {
				pot.sort = SORT_RAM
			}
			if kk == 'I' {
				pot.reverse = !pot.reverse
			}
		case <-t:
			pot.snapshot = pot.Snapshot()
		case <-s:
			gnc.End()
			pot.win.Refresh()
		}
	}
}

func NewPot() *Pot {
	// default settings
	return &Pot{
		c:                   initClient(),
		status:              STATUS_POT,
		snapshot:            []Container{},
		win:                 nil,
		showGlobalProcesses: false, // show processes
		reverse:             false, // non-reversed sort
		sort:                SORT_CPU,
	}
}

func main() {

	flag.Parse()

	p := NewPot()
	p.Run()
}

type ItemPair struct {
	Com string
	Def string
}

type Page struct {
	Header string
	Info   string
	Body   []ItemPair
	Footer string
}

var help = Page{
	Header: `
Help of "pot" command:
`,
	Info: `
`,
	Body: []ItemPair{
		{"<arrow up>", "scroll up"},
		{"<arrow down>", "scroll down"},
		{"<space>", "select/unselect container"},
		{"u", "unselect all containers"},
		{"q", "quit"},
		{"h", "prints this help"},
		{"a", "show/hide processes on selected containers"},
		{"A", "show/hide processes on all containers"},
		{"k", "kill selected containers"},
		{"s", "start selected containers"},
		{"S", "stop selected containers"},
		{"r", "remove selected containers"},
		{"i", "view information about current container"},
		{"p", "pause selected containers"},
		{"P", "unpause selected containers"},
		{"1", "sort by name"},
		{"2", "sort by image"},
		{"3", "sort by id"},
		{"4", "sort by command"},
		{"5", "sort by uptime"},
		{"6", "sort by status"},
		{"7", "sort by %CPU"},
		{"8", "sort by %RAM"},
		{"I", "revert current sort"},
	},
	Footer: `
Press 'h' to return.
`,
}
