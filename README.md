# dotfile-linker

Keeps a dotfiles git repo and your home directory in sync using symlinks,
without ever silently clobbering a file that has local changes.

## The problem

The usual dotfiles setup: a git repo with files like `bashrc`, `gitconfig`,
`vimrc`, and a step that links each one into `$HOME` as `.bashrc`,
`.gitconfig`, `.vimrc`. That step is easy to get wrong in two ways:

- It blindly overwrites whatever is already at the target, including a
  `.bashrc` you edited by hand three months ago and forgot to commit back.
- Naive implementations read whole files into memory to compare them,
  which is wasteful and occasionally a real problem if something large
  ended up in the dotfiles directory (a shell history export, a vendored
  binary, whatever).

`dotlink` avoids the first by refusing to touch a target file unless it's
either already a symlink to the source or byte-for-byte identical to it.
It avoids the second by hashing files in fixed-size chunks (see
`hash.go`) instead of loading them whole.

## Layout convention

A file named `bashrc` in the source directory maps to `~/.bashrc`. Only
top-level files are considered; subdirectories and dotfiles (like `.git`)
in the source are skipped.

## Custom targets

Not everything fits "source name with a dot in front" — `gitconfig`
might belong at `~/.config/git/config` instead of `~/.gitconfig`. Put
overrides in a mapping file, one `name target` pair per line, target
relative to the target directory:

```
# .dotlinkmap
gitconfig config/git/config
```

The CLI loads `<source>/.dotlinkmap` automatically if it exists, or a
different file via `-map`. Library callers pass a mapping (or `nil`)
straight to `Plan`/`PlanUnlink`; `dotlink.LoadMapping` reads the file
format above.

## Library usage

```go
mapping, err := dotlink.LoadMapping(filepath.Join(source, ".dotlinkmap"))
if err != nil {
    log.Fatal(err)
}

actions, err := dotlink.Plan("/home/ryan/dotfiles", os.Getenv("HOME"), mapping)
if err != nil {
    log.Fatal(err)
}

for _, a := range actions {
    if a.Kind == dotlink.Conflict {
        fmt.Println("needs manual review:", a.Target)
        continue
    }
    if err := dotlink.Apply(a); err != nil {
        log.Fatal(err)
    }
}
```

## CLI usage

```
# show what would happen, without touching anything
dotlink status -source ~/dotfiles -target ~

# create or repair symlinks, skipping anything that would overwrite
# a file with different content
dotlink link -source ~/dotfiles -target ~

# same, but move conflicting files aside (to "<target>.bak", or
# "<target>.bak.N" if that's taken) instead of skipping them
dotlink link -source ~/dotfiles -target ~ -backup

# remove symlinks dotlink created, leaving anything else at the
# target path alone
dotlink unlink -source ~/dotfiles -target ~

# use a mapping file somewhere other than <source>/.dotlinkmap
dotlink link -source ~/dotfiles -target ~ -map ~/dotfiles/custom.map
```

`-source` and `-target` default to `.` and `$HOME`, so from inside a
dotfiles checkout `dotlink link` is usually enough.

Build it with:

```
go build -o dotlink ./cmd/dotlink
```

## Status

Early skeleton. Works for the basic link/relink/conflict cases described
above, plus removing symlinks it created via `unlink`, backing up
conflicting files with `-backup`, and custom targets via a mapping file.
Not yet handled: recursive source directories, and no tests yet.
