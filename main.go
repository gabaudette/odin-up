package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"

	"odin-up/internal/githubclient"
	"odin-up/internal/odin"
	"odin-up/internal/system"
	"odin-up/internal/tui"
)

const version = "0.1.1"

const usage = `odin-up - install, update and manage the Odin compiler

Usage:
  odin-up install     Install the latest Odin release
  odin-up update      Update Odin to the latest release
  odin-up status      Show the current installation status
  odin-up uninstall   Remove the odin-up managed installation
  odin-up             Launch the interactive menu

Options:
  -v, --version    Show the version
  -h, --help       Show this help
`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if err := system.CheckOS(runtime.GOOS); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}

	if len(args) == 0 {
		return tui.Run(tui.OpMenu)
	}

	switch args[0] {
	case "-h", "--help", "help":
		fmt.Print(usage)
		return 0
	case "-v", "--version", "version":
		fmt.Printf("odin-up %s\n", version)
		return 0
	case "install":
		return withLock(tui.OpInstall)
	case "update":
		return withLock(tui.OpUpdate)
	case "uninstall":
		return withLock(tui.OpUninstall)
	case "status":
		return runStatus()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", args[0], usage)
		return 1
	}
}

func withLock(op tui.OpKind) int {
	lock, err := system.AcquireLock("/tmp/odin-up.lock")

	if err != nil {
		if errors.Is(err, system.ErrAnotherRunning) {
			fmt.Fprintln(os.Stderr, "Another odin-up operation is currently running.")
		} else {
			fmt.Fprintf(os.Stderr, "failed to acquire lock: %v\n", err)
		}
		return 1
	}

	defer lock.Release()

	return tui.Run(op)
}

func runStatus() int {
	inst := &odin.Installer{Client: githubclient.New(), Runner: system.PlainRunner{}}
	st, err := inst.Status(context.Background())

	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: "+err.Error())
		return 1
	}

	fmt.Print(odin.FormatStatus(st))

	return 0
}
