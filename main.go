package main

import (
	"flag"
	"strings"
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

var (
	active = 0
	scroll = 0
)

func main() {

	flag.Parse()

	p := NewPot()
	p.Run()
}
