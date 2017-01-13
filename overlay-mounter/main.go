package main

import (
	"fmt"
	"os"
	"time"

	"github.com/urfave/cli"
)

var (
	// Name is the name of the application
	Name = "box"
	// Email is my email
	Email = "github@hollensbe.org"
	// Usage is the title of the application
	Usage = "Advanced mruby Container Image Builder"
	// Author is me
	Author = "Erik Hollensbe"

	// Copyright is the copyright, generated automatically for each year.
	Copyright = fmt.Sprintf("(C) %d %s - Licensed under MIT license", time.Now().Year(), Author)
	// UsageText is the description of how to use the program.
	UsageText = "box [options] filename"
)

func main() {
	app := cli.NewApp()
	app.Name = Name
	app.Email = Email
	app.Version = Version
	app.Usage = Usage
	app.Author = Author
	app.Copyright = Copyright
	app.UsageText = UsageText
	app.HideHelp = true

	app.Action = func(ctx *cli.Context) {
		for _, arg := range ctx.Args() {
		}
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, err.Error())
		os.Exit(1)
	}
}
