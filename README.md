# odin-up

Install, update, inspect and uninstall the [Odin](https://odin-lang.org) compiler on Linux, from the official GitHub releases, with an interactive terminal UI.

## Features

- Fetches the latest official Odin release from the `odin-lang/Odin` GitHub repository.
- Downloads the Linux archive matching your architecture (`amd64` / `arm64`).
- Installs into a versioned layout under `/opt/odin` and exposes `odin` via `/usr/local/bin/odin`.
- Offers to adopt an existing Odin installation into the managed layout instead of downloading a fresh copy.
- Interactive menu, install progress, and confirmations built on Bubble Tea.
- Privileged operations run through `sudo`, so `odin-up` never needs to run as root.
- Update is safe: the currently installed version is left untouched until the new one has been downloaded, extracted and validated, then swapped atomically.
- Never overwrites a `/usr/local/bin/odin` it does not own.
- Prevents concurrent operations from running at the same time.

## Requirements

- Linux
- `sudo` access (used only for installing into `/opt` and linking into `/usr/local/bin`)
- Required packages are detected automatically and can be installed from within the tool:
  - `curl` (release download)
  - `tar` (archive extraction)
  - `gcc` / `build-essential`, `clang`, `llvm` and `git` (Odin build prerequisites)

## Installation

Build from source:

```sh
go build -o odin-up .
```

Optionally install it somewhere on your `PATH`:

```sh
sudo install -m 0755 odin-up /usr/local/bin/odin-up
```

## Usage

Run `odin-up` with no arguments to open the interactive menu, or use a subcommand:

```
odin-up install     Install the latest Odin release
odin-up update      Update Odin to the latest release
odin-up status      Show the current installation status
odin-up uninstall   Remove the odin-up managed installation
odin-up             Launch the interactive menu
```

Options:

```
-v, --version    Show the version
-h, --help       Show this help
```

`install` and `uninstall` ask for confirmation before doing anything; you can answer with the arrow keys or `j`/`k` and confirm with Enter. On the menu, `Enter` selects, `q` quits.

## Layout

A managed installation looks like this:

```
/opt/odin
├── current/            symlink to the active version
└── versions/
    └── odin-linux-amd64-dev-2026-08/
        ├── odin        the compiler binary
        └── core/       the core library

/usr/local/bin/odin -> /opt/odin/current/odin
```

## Safety

- Downloads and extractions are validated before the active installation is changed.
- Archive extraction rejects traversal and absolute paths and never creates symlinks from the archive.
- Only directories that `odin-up` itself manages are ever removed.
- If an update fails, the previously installed version remains active.
