package main

import "strconv"

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
