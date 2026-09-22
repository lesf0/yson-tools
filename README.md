# yson-tools

Command-line tools for the Yandex YSON format. `yson-convert` converts values
between YSON and JSON, `ysonq` runs jq programs over YSON, `ysondiff` diffs two
YSON documents and `yson-format` pretty-prints a YSON file in place.

## Installation

All four commands are installed together by every method below. `go install`,
being one command per module, gets you the two Go programs and not the two shell
scripts.

### Homebrew

Works on macOS and Linux (amd64 and arm64):

```bash
brew install lesf0/tap/yson-tools
```

The formula also installs its own copy of `jsondiff`, so `ysondiff` works
without any further setup.

The fully qualified name is enough to install: since Homebrew 6.0.0 non-official
taps need to be [trusted](https://docs.brew.sh/Tap-Trust), and naming the formula
in full trusts exactly that formula. If you prefer to `brew tap lesf0/tap` and
then use the short name, trust it first:

```bash
brew trust --formula lesf0/tap/yson-tools
```

### Arch Linux

```bash
yay -S yson-tools   # or any other AUR helper
```

### Debian / Ubuntu and Fedora / RHEL

Prebuilt `.deb` and `.rpm` packages (amd64 and arm64) are attached to every
[release](https://github.com/lesf0/yson-tools/releases):

```bash
# Debian / Ubuntu
sudo apt install ./yson-tools_0.3.6-1_amd64.deb

# Fedora / RHEL
sudo dnf install ./yson-tools-0.3.6-1.x86_64.rpm
```

### Go

`yson-convert` and `ysonq` are Go programs; the other two are shell scripts and
come with the packages and tarballs. The two share a library (`ysonlib`, which
also holds the formatter `vscode-yson` uses), and that library lives in this
repository rather than being a module of its own on the Go proxy, so both are
built from a clone:

```bash
git clone https://github.com/lesf0/yson-tools
cd yson-tools
for module in yson-convert ysonq; do (cd "$module" && go install .); done
```

### Tarballs

The release also carries `yson-tools-<version>-<os>-<arch>.tar.gz` for
linux/darwin on amd64/arm64; these are what the Homebrew formula installs, and
they work anywhere with a `bash`:

```bash
tar -xzf yson-tools-0.3.6-linux-amd64.tar.gz
install -m755 yson-tools-0.3.6-linux-amd64/* ~/.local/bin/
```

### Requirements

`yson-convert` and `ysonq` are self-contained binaries; `yson-format` and
`ysondiff` are shell scripts that call them. `ysondiff` needs `jdiff` from
`jsondiff` (`python-jsondiff` on Arch, `python3-jsondiff` on
Debian/Ubuntu/Fedora), so the `.deb`/`.rpm` packages recommend it and the
Homebrew formula bundles its own copy; it falls back to `jsondiff-jdiff` and
`python3 -m jsondiff.cli` where that name is packaged differently, and tells you
what to install if it finds none. Nothing needs `jq`.

## Usage

### yson-convert

A tool to convert values between YSON and JSON.

Usage: `yson-convert [-m mode] [-f format] [-seq] value` or `echo value | yson-convert [-m mode] [-f format] [-seq]`

The value comes from the argument, from stdin (a pipe, a redirect or a
here-document), or from `-i FILE`; the result goes to stdout or to `-o FILE`,
with `-` meaning stdin and stdout. `-o` opens the file only after the input has
been read and converted, so a file can be formatted in place, keeping its inode
and its permissions:

```bash
$ echo '{foo=bar}' > f.yson
$ yson-convert -m pretty -i f.yson -o f.yson
$ cat f.yson
{
    "foo" = "bar";
}
```

Flags go before the value: `yson-convert {a=1} -o out.yson` fails, since
everything after the value is read as another argument.

Modes:

- y2j: convert YSON to JSON
- j2y: convert JSON to YSON
- pretty: reformat YSON
- guess [default]: try y2j, in case of failure fallback to j2y

Formats:

- pretty [default]: print YSON/JSON text form with intendations
- compact: print YSON/JSON text form in one line
- binary: print YSON binary form, won't work with JSON

Sequence of YSON (aka YSONL):

`-seq` flag allows to parse a sequence of YSON/JSON values rather than a singular YSON/JSON value. It uses binary search to determine the bounds of separate YSON objects and is not *that* effective, but it works. It needs an explicit mode: in `guess` mode a truncated value cannot be told apart from an invalid one.

Example: 
```bash
$ echo "{foo={bar=<q=e>%true;baz=qqq}}" | yson-convert -m y2j
{
	"foo": {
		"bar": {
			"Attrs": {
				"q": "e"
			},
			"Value": true
		},
		"baz": "qqq"
	}
}
```

### yson-format

A shorthand script to apply pretty formatter to an YSON file: it is
`yson-convert -m pretty -i FILE -o FILE`, so the file is formatted in place and
keeps its permissions.

Example:
```bash
$ echo '{foo={bar=<q=e>%true;baz=qqq}}' > my-yson-file.yson
$ yson-format my-yson-file.yson
$ cat my-yson-file.yson
{
    "foo" = {
        "bar" = <
            "q" = "e";
        > %true;
        "baz" = "qqq";
    };
}
```

### ysonq

`ysonq` is `jq` for YSON: it reads YSON values, runs a jq program over each of
them, and writes the result back as YSON.

It is not a wrapper around `jq`: the language comes from
[gojq](https://github.com/itchyny/gojq) and the format handling from the same
library `yson-convert` uses, so nothing is converted through JSON on the way and
no `jq` has to be installed. Values JSON cannot hold survive the trip, and a
document that does not parse is reported with its place in the input.

A value carrying attributes is the `Attrs`/`Value` map `yson-convert` writes, so
`.foo.bar.Attrs.q` is the attribute `q` of `.foo.bar`. Attributes are written by
building that map — with `.foo` itself when nothing else is on the value, or on
the existing `.foo.Attrs` when something is:

```bash
$ echo '{foo=1}' | ysonq -c '.foo = {Attrs:{q:"e"}, Value:.foo}'
{foo=<q=e;>1;}

$ echo '{foo=<q=e>1}' | ysonq -c '.foo.Attrs.q = "w"'
{foo=<q=w;>1;}
```

The first form replaces the whole map, so it drops any attribute that was there;
the second keeps them. A YSON map of your own that happens to have `Attrs` and
`Value` keys reads as the attributed value it looks like — the same ambiguity
`yson-convert` has.

The input is a stream of YSON documents separated by whitespace or `;`, so a
file with one value per line works as it is. `--seq` reads the RS-separated form
(a `\x1e` before each document) and writes it back, `--stream` reads `[path,
leaf]` pairs, and `-R` reads lines of text instead of YSON.

The output is always YSON: pretty-printed with four spaces per level unless `-c`
asks for one value per line, with the keys of an object in order (`-S` is
accepted and does nothing, as they are always in order), coloured only when
writing to a terminal unless `-C`/`-M` or the toolkit's colour variables say
otherwise. An integer that does not fit into YSON's 64 bits is refused rather
than rounded off, and `--raw-output0` refuses a string containing a NUL rather
than writing a stream nothing can read back.

`ysonq --help` lists the flags, which are `jq`'s, including `--argjson`,
`--argyson`, `--ysonargs`, `--slurpfile`, `--rawfile`, `-L` for modules, `-f` and
`-e`. The four that are not implemented — `--yaml-input`, `--yaml-output`,
`--stream-errors` and `-a`/`--ascii-output` — are refused with an error instead
of being quietly ignored.

The exit codes are `jq`'s: 0 when the run succeeded, 1 with `-e` if the last
value was false or null, 2 for a command line that cannot be used (an input file
that cannot be opened included), 3 for a program that does not compile, 4 with
`-e` if nothing was output at all, and 5 for a document that does not parse or a
program that fails while running.

Examples :

```bash
# Get field by path
$ echo "{foo={bar=<q=e>%true;baz=qqq}}" | ysonq '.foo'
{
    "bar" = <
        "q" = "e";
    > %true;
    "baz" = "qqq";
}

# Get attribute by path
$ echo "{foo={bar=<q=e>%true;baz=qqq}}" | ysonq '.foo.bar.Attrs.q'
"e"

# Print raw literals (won't be converted back to YSON)
$ echo "[q;w;e;r;t;y]" | ysonq -r '.[]'
q
w
e
r
t
y

# JSONL (will be represented as YSONL, which is not really a thing but will be parsed back)
$ echo "[<q=w>e;<r=t>y]" | ysonq '.[]'
<
    "q" = "w";
> "e"
<
    "r" = "t";
> "y"

# Compact form
$ echo "{foo={bar=<q=e>%true;baz=qqq}}" | ysonq -c '.foo'
{bar=<q=e;>%true;baz=qqq;}

# YSONL compact form
$ echo "[<q=w>e;<r=t>y]" | ysonq -c '.[]'
<q=w;>e
<r=t;>y

# --ysonargs
$ ysonq -n --ysonargs '$ARGS.positional' -c '[foo;bar;]' '%false' '<q=e>#'
[[foo;bar;];%false;<q=e;>#;]

# --argyson
$ ysonq -n '$ARGS.named' -c --argyson first '[foo;bar;]' --argyson second '%false' --argyson third '<q=e>#'
{first=[foo;bar;];second=%false;third=<q=e;>#;}

# --slurp
$ seq 1 5 | ysonq -s
[
    1;
    2;
    3;
    4;
    5;
]

# Values JSON cannot hold come back as themselves
$ ysonq -n -c 'nan, infinite, 18446744073709551615'
%nan
%inf
18446744073709551615u
```

### ysondiff

A wrapper script for jdiff.

Examples:

```bash
$ ysondiff <(echo '{foo=<q=w>baz}') <(echo '{foo=<q=e>bar}') -i 4 -s symmetric
{
    "foo" = <
        "q" = [
            "w";
            "e";
        ];
    > [
        "baz";
        "bar";
    ];
}
```

## Testing

`.github/workflows/test.yml` runs the Go tests of every module and every example
in the Usage section of this file. The release workflow calls it first, so
nothing is packaged or published unless these pass. The examples are checked by
`scripts/readme-examples.py`, which extracts the `$ command` lines and compares
the output with what this README claims, so changing what a tool prints means
updating this file:

```bash
for module in yson-convert ysonq; do (cd "$module" && go build -o ../build/ .); done
./scripts/readme-examples.py
```

The examples need `jsondiff` (which ships `jdiff`), the same requirement
`ysondiff` itself has, and look for the tools in `./build` and next to this file.
The Go tests are run per module, since the tree holds four of them:
pretty-formatter, ysonlib, yson-convert and ysonq.

## Releasing

Publishing a GitHub release (`git tag v0.3.6 && git push origin v0.3.6`, then a
release from that tag) runs `.github/workflows/release.yml`, which:

1. runs the tests and the README examples — the same checks a push gets — and
   stops there if they fail,
2. builds `yson-convert` and `ysonq` for linux/darwin on amd64/arm64 with
   `CGO_ENABLED=0` (`ysonq` with `-X main.version=<tag>`, which is what
   `ysonq --version` reports) and packs them with the two shell scripts into
   `yson-tools-<version>-<os>-<arch>.tar.gz`,
3. builds `.deb` and `.rpm` packages from `nfpm.yaml` for amd64 and arm64, and
   attaches them and the tarballs to the release,
4. renders `packaging/homebrew/yson-tools.rb.tmpl` with the new version and the
   four tarball checksums, and pushes the result to `lesf0/homebrew-tap` as
   `Formula/yson-tools.rb`,
5. bumps `pkgver` in `packaging/aur/PKGBUILD`, regenerates `.SRCINFO` and pushes
   both to [the AUR package](https://aur.archlinux.org/packages/yson-tools).

Only *published* releases trigger this (a draft builds nothing). To rehearse the
whole thing without publishing, run the workflow manually
(`workflow_dispatch`) with an existing tag: it builds and renders everything,
but attaches nothing to a release and pushes nowhere.

Workflows are read from the tree being released rather than from `main`, so a
tag has to contain both of them: a tag from before `test.yml` existed cannot be
released, since the release workflow would call a file that is not in its tree.

Everything published is generated from files in this repository — `nfpm.yaml`,
`packaging/homebrew/yson-tools.rb.tmpl` and `packaging/aur/PKGBUILD` are the
sources of truth, and the AUR and the tap only ever receive rendered copies of
them. Edit them here, not there.

Two repository secrets are required:

| Secret | What it is |
| --- | --- |
| `HOMEBREW_TAP_TOKEN` | fine-grained PAT with `Contents: read and write` on `lesf0/homebrew-tap`; the tap repository has to exist |
| `AUR_SSH_PRIVATE_KEY` | SSH private key whose public half is registered in the AUR account |
