package main

import (
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

type Top struct {
	client              *docker.Client // Used to talk to the daemon
	status              Status         // Current status
	snapshot            []Container    // Current containers/processes state
	win                 *gnc.Window    // goncurse Window
	showGlobalProcesses bool           // whether or not to show processes
	reverse             bool           // Reverse sort
	sort                Sort           // Current sort
	currentInfo         Container
}

// Returns the running processes for the current Container
func (top *Top) GetProcesses(cid string) []ProcessLine {
	res := make([]ProcessLine, 0, 10)

	topr, e := top.client.TopContainer(cid, url.QueryEscape("xo pid,etime,%cpu,%mem,cmd"))
	if e != nil {
		log.Println(e)
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
func (top *Top) Snapshot() []Container {
	res := make([]Container, 0, 16)

	cnts, e := top.client.ListContainers(docker.ListContainersOptions{All: true})
	if e != nil {
		log.Println(e)
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
			c.processes = top.GetProcesses(c.container.Id)
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

		for _, cn := range top.snapshot {
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

func (top *Top) PrintActive(l PrintedLine, lc int, i int) {
	if i < scroll || i >= scroll+lc {
		return
	}
	if active == i {
		top.win.AttrOn(gnc.A_REVERSE)
		top.win.Println(l.line)
		top.win.AttrOff(gnc.A_REVERSE)
	} else {
		if l.isActive {
			top.win.ColorOn(COLOR_SELECTION)
			top.win.Println(l.line)
			top.win.ColorOff(COLOR_SELECTION)
		} else if l.isContainer {
			if l.isRunning {
				top.win.ColorOn(COLOR_RUNNING)
				top.win.Println(l.line)
				top.win.ColorOff(COLOR_RUNNING)
			} else {
				top.win.ColorOn(COLOR_CONTAINER)
				top.win.Println(l.line)
				top.win.ColorOff(COLOR_CONTAINER)
			}
		} else {
			top.win.Println(l.line)
		}
	}
}

func NewTop() *Top {
	// default settings
	return &Top{
		client:              initClient(),
		status:              STATUS_TOP,
		snapshot:            []Container{},
		win:                 nil,
		showGlobalProcesses: false, // show processes
		reverse:             false, // non-reversed sort
		sort:                SORT_CPU,
	}
}

func (top *Top) Run() {
	var err error

	top.win, err = gnc.Init()
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
	top.win.Keypad(true)
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
	}(top.win, k)

	top.snapshot = top.Snapshot()

	for {
		// Print screen
		my, mx := top.win.MaxYX()
		lc := my - HEADER_SIZE
		wc := (mx - 1) / NB_COLUMNS
		top.win.Erase()
		if mx < 40 || my < 5 {
			continue
		}

		switch top.status {
		case STATUS_TOP:
			top.Printtop(wc, lc)
			top.win.Refresh()
		case STATUS_HELP:
			top.PrintHelp(wc)
			top.win.Refresh()
		case STATUS_INFO:
			top.PrintInfo(wc)
			top.win.Refresh()
		}

		// Handle Events
		select {
		case kk := <-k:
			if kk == 'q' {
				return
			}
			switch top.status {
			case STATUS_TOP:
				if kk == gnc.KEY_DOWN {
					active = active + 1
				}
				if kk == gnc.KEY_UP {
					active = active - 1
				}
				if kk == 'h' {
					top.status = STATUS_HELP
				}
				if kk == 'A' {
					top.showGlobalProcesses = !top.showGlobalProcesses
				}
				if kk == 'k' {
					for _, c := range top.getSelectedContainers() {
						top.KillContainer(&top.snapshot[c])
					}
				}
				if kk == 'u' {
					for i, _ := range top.snapshot {
						top.snapshot[i].isSelected = false
					}
				}
				if kk == 'a' {
					for _, c := range top.getSelectedContainers() {
						top.snapshot[c].showProcesses = !top.snapshot[c].showProcesses
					}
				}
				if kk == ' ' {
					c := top.GetContainerByPos(active)
					if c != -1 {
						top.snapshot[c].isSelected = !top.snapshot[c].isSelected
					}
					active = active + 1
				}
				if kk == 's' {
					for _, c := range top.getSelectedContainers() {
						top.StartContainer(&top.snapshot[c])
					}
				}
				if kk == 'S' {
					for _, c := range top.getSelectedContainers() {
						top.StopContainer(&top.snapshot[c])
					}
				}
				if kk == 'r' {
					for _, c := range top.getSelectedContainers() {
						top.RmContainer(&top.snapshot[c])
					}
				}
				if kk == 'i' {
					c := top.GetContainerByPos(active)
					if c != -1 {
						top.currentInfo = top.snapshot[c]
						top.status = STATUS_INFO
					}
				}
				if kk == 'p' {
					for _, c := range top.getSelectedContainers() {
						top.PauseContainer(&top.snapshot[c])
					}
				}
				if kk == 'P' {
					for _, c := range top.getSelectedContainers() {
						top.UnpauseContainer(&top.snapshot[c])
					}
				}
			case STATUS_HELP:
				if kk == 'h' {
					top.status = STATUS_TOP
				}
			case STATUS_INFO:
				if kk == 'i' {
					top.status = STATUS_TOP
				}
			}
			if kk == '1' {
				top.sort = SORT_NAME
			}
			if kk == '2' {
				top.sort = SORT_IMAGE
			}
			if kk == '3' {
				top.sort = SORT_ID
			}
			if kk == '4' {
				top.sort = SORT_COMMAND
			}
			if kk == '5' {
				top.sort = SORT_UPTIME
			}
			if kk == '6' {
				top.sort = SORT_STATUS
			}
			if kk == '7' {
				top.sort = SORT_CPU
			}
			if kk == '8' {
				top.sort = SORT_RAM
			}
			if kk == 'I' {
				top.reverse = !top.reverse
			}
		case <-t:
			top.snapshot = top.Snapshot()
		case <-s:
			gnc.End()
			top.win.Refresh()
		}
	}
}

func (top *Top) Updatetop(lc int, wc int) {
	ss := make([]PrintedLine, 0, 42)

	sort.Sort(SortableContainers{top.snapshot, top.sort, top.reverse})

	for _, cnt := range top.snapshot {
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
		if top.showGlobalProcesses || cnt.showProcesses {
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
		top.PrintActive(s, lc, i)
	}
}

func (top *Top) colorColumn(s string, sort Sort) {
	if top.sort == sort {
		top.win.AttrOn(gnc.A_REVERSE)
		top.win.ColorOn(COLOR_HIGHLIGHT)
		top.win.Printf("%s", s)
		top.win.ColorOff(COLOR_HIGHLIGHT)
		top.win.AttrOff(gnc.A_REVERSE)
		return
	}
	top.win.AttrOn(gnc.A_REVERSE)
	top.win.Printf("%s", s)
	top.win.AttrOff(gnc.A_REVERSE)
}

func (top *Top) PrintHeader(wc int) {
	o, _ := exec.Command("uptime").Output()
	top.win.Printf("%s", o)

	top.colorColumn(PrettyColumn("Name", wc, " ", " "), SORT_NAME)
	top.colorColumn(PrettyColumn("Image", wc, " ", " "), SORT_IMAGE)
	top.colorColumn(PrettyColumn("Id", wc, " ", " "), SORT_ID)
	top.colorColumn(PrettyColumn("Command", wc, " ", " "), SORT_COMMAND)
	top.colorColumn(PrettyColumn("Uptime", wc, " ", " "), SORT_UPTIME)
	top.colorColumn(PrettyColumn("Status", wc, " ", " "), SORT_STATUS)
	top.colorColumn(PrettyColumn("%CPU", wc, " ", " "), SORT_CPU)
	top.colorColumn(PrettyColumn("%RAM", wc, " ", " "), SORT_RAM)
	top.win.Println()
}

func (top *Top) Printtop(wc int, lc int) {
	top.PrintHeader(wc)
	top.Updatetop(lc, wc)
}

func (top *Top) PrintInfo(wc int) {
	var info = Page{
		Header: "\nInformation about container:\n",
		Info:   "",
		Body:   []ItemPair{},
		Footer: "\nPress 'i' to return.",
	}

	info.Body = append(info.Body, ItemPair{"Name", top.currentInfo.container.Name})
	info.Body = append(info.Body, ItemPair{"Id", top.currentInfo.container.Id})
	info.Body = append(info.Body, ItemPair{"Command", top.currentInfo.container.Command})
	info.Body = append(info.Body, ItemPair{"Uptime", top.currentInfo.container.Uptime})
	info.Body = append(info.Body, ItemPair{"Status", top.currentInfo.container.Status})
	info.Body = append(info.Body, ItemPair{"%CPU", top.currentInfo.container.CPU})
	info.Body = append(info.Body, ItemPair{"%RAM", top.currentInfo.container.RAM})

	top.PrintPage(info, wc)
}

func (top *Top) PrintPage(p Page, wc int) {
	top.win.ColorOn(COLOR_SELECTION)
	top.win.Printf("%s\n", p.Header)
	top.win.ColorOff(COLOR_SELECTION)

	top.win.Printf("%s", p.Info)

	for _, v := range p.Body {
		top.win.ColorOn(COLOR_HELP)
		top.win.Printf("%s", PrettyColumn(v.Com+":", 20, " ", " "))
		top.win.ColorOff(COLOR_HELP)
		top.win.Printf("%s", PrettyColumn(v.Def, 40, " ", " "))
		top.win.Println()
	}

	top.win.ColorOn(COLOR_CONTAINER)
	top.win.Printf("%s\n", p.Footer)
	top.win.ColorOff(COLOR_CONTAINER)
}

func (top *Top) PrintHelp(wc int) {
	top.PrintPage(help, wc)
}

func (top *Top) GetContainerByPos(line_num int) int {
	i := 0

	for res, cnt := range top.snapshot {
		if i == line_num {
			return res
		}
		if i > line_num {
			break
		}
		if top.showGlobalProcesses || cnt.showProcesses {
			i += len(cnt.processes)
		}
		i++
	}

	return -1
}

func (top *Top) getSelectedContainers() []int {
	res := make([]int, 0, 5)
	for i, c := range top.snapshot {
		if c.isSelected {
			res = append(res, i)
		}
	}
	if len(res) == 0 {
		c := top.GetContainerByPos(active)
		if c != -1 {
			if !top.snapshot[c].isSelected {
				res = append(res, c)
			}
		}
	}
	return res
}

func (top *Top) StopContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "stop", id).Run()
	}(id)
}

func (top *Top) StartContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "start", id).Run()
	}(id)
}

func (top *Top) RmContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "rm", id).Run()
	}(id)
}

func (top *Top) KillContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "kill", id).Run()
	}(id)
}

func (top *Top) PauseContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "pause", id).Run()
	}(id)
}

func (top *Top) UnpauseContainer(c *Container) {
	id := c.container.Id
	go func(id string) {
		// @todo: use docker API
		exec.Command("docker", "unpause", id).Run()
	}(id)
}
