// Copyright 2015 Ghislain Guiot <gt.ghislain@gmail.com>. All rights reserved.
// Use of this source code is governed by a MIT license that can
// be found in the LICENSE file.

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/fsouza/go-dockerclient"
	"github.com/ghigt/termutil"
	"github.com/nsf/termbox-go"
)

func display() error {

	termutil.Screen.EventFunc = func(ev termbox.Event) {
		switch ev.Type {
		case termbox.EventKey:
			switch ev.Key {
			case termbox.KeyEsc:
				termutil.Quit()
			}
		}
	}

	win := termutil.NewWindow()

	win.UpdateFunc = func() []string {
		return []string{}
	}
	head := win.NewSubWindow()
	head.SizeX = 10
	head.SizeY = 1
	head.UpdateFunc = func() []string {
		return []string{"dtop 0.0.0alpha"}
	}
	body := win.NewSubWindow()
	body.Y = 1

	client, err := docker.NewClientFromEnv()
	if err != nil {
		return err
	}

	body.UpdateFunc = func() []string {
		cnts, err := client.ListContainers(docker.ListContainersOptions{All: true})
		if err != nil {
			termutil.Quit()
			//log.Fatal(err)
		}

		lcnts := []string{}
		for _, cnt := range cnts {
			lcnts = append(lcnts, fmt.Sprintf("%s", cnt.Names[0]))
		}
		return lcnts
	}

	return nil
}

func main() {

	var err error
	err = termutil.Init(time.Second)
	if err != nil {
		log.Fatal(err)
	}

	err = display()
	if err != nil {
		termutil.End()
		log.Fatal(err)
	}

	err = termutil.Run()
	termutil.End() // Ends before printing

	if err != nil {
		log.Fatal(err)
		return
	}
}
