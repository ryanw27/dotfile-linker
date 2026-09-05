// Command dotlink is a thin wrapper around the dotlink library: it prints
// or applies the plan for linking a dotfiles directory into $HOME.
package main

import (
	"flag"
	"fmt"
	"os"

	dotlink "github.com/ryanw27/dotfile-linker"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}

	cmd := os.Args[1]
	fs := flag.NewFlagSet(cmd, flag.ExitOnError)
	source := fs.String("source", ".", "directory containing the dotfiles to link")
	target := fs.String("target", os.Getenv("HOME"), "directory the symlinks are created in")
	backup := fs.Bool("backup", false, "for link: move conflicting files aside instead of skipping them")
	fs.Parse(os.Args[2:])

	switch cmd {
	case "status":
		runStatus(*source, *target)
	case "link":
		runLink(*source, *target, *backup)
	case "unlink":
		runUnlink(*source, *target)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: dotlink <status|link|unlink> [-source dir] [-target dir] [-backup]")
}

func runStatus(source, target string) {
	actions, err := dotlink.Plan(source, target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dotlink:", err)
		os.Exit(1)
	}
	for _, a := range actions {
		fmt.Printf("%-12s %-20s %s\n", a.Kind, a.Name, a.Target)
	}
}

func runLink(source, target string, backup bool) {
	actions, err := dotlink.Plan(source, target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dotlink:", err)
		os.Exit(1)
	}

	conflicts := 0
	for _, a := range actions {
		if a.Kind == dotlink.Conflict && !backup {
			fmt.Fprintf(os.Stderr, "skip %s: %s already exists with different content\n", a.Name, a.Target)
			conflicts++
			continue
		}

		apply := dotlink.Apply
		if backup {
			apply = dotlink.ApplyBackup
		}
		if err := apply(a); err != nil {
			fmt.Fprintln(os.Stderr, "dotlink:", err)
			os.Exit(1)
		}
		if a.Kind == dotlink.Conflict {
			fmt.Printf("backup   %s\n", a.Name)
		} else if a.Kind != dotlink.NoOp {
			fmt.Printf("%-8s %s\n", a.Kind, a.Name)
		}
	}

	if conflicts > 0 {
		fmt.Fprintf(os.Stderr, "\n%d file(s) left untouched, resolve manually and rerun (or pass -backup)\n", conflicts)
		os.Exit(1)
	}
}

func runUnlink(source, target string) {
	actions, err := dotlink.PlanUnlink(source, target)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dotlink:", err)
		os.Exit(1)
	}

	for _, a := range actions {
		if a.Kind == dotlink.Foreign {
			fmt.Fprintf(os.Stderr, "skip %s: %s is not a symlink to %s\n", a.Name, a.Target, a.Source)
			continue
		}
		if err := dotlink.ApplyUnlink(a); err != nil {
			fmt.Fprintln(os.Stderr, "dotlink:", err)
			os.Exit(1)
		}
		if a.Kind == dotlink.Managed {
			fmt.Printf("removed  %s\n", a.Name)
		}
	}
}
