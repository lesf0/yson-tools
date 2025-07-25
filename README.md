## CLI Tools for Yandex YSON manipulation

### yson-convert

A tool to convert values between YSON and JSON.

Installation: `go install github.com/lesf0/yson-tools/yson-convert@latest`

Usage: `yson-convert [-m mode] [-f format] [-seq] value` or `echo value | yson-convert [-m mode] [-f format] [-seq]`

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

`-seq` flag allows to parse a sequence of YSON/JSON values rather than a singular YSON/JSON value. It uses binary search to determine the bounds of separate objects and is not *that* effective, but it works.

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

Note: JSON representation produced by y2j mode differs from JSON representation required by j2y mode because reasons. It's neatly hacked around in `ysonq` code, please use `ysonq` if you need back-and-forth conversion.

### ysonq

A wrapper script for `jq` which converts input stream to JSON and back via `yson-convert`, effectively allowing to use `jq` with YSON with almost no downsides.

By default, `ysonq` tries to convert `jq`'s output back to YSON / YSON*L*, but won't do so if it seems impossible (i.e. -r string literals).

Some of `jq`'s flags (namely, format/stream stuff) are not supported yet, I'd like to support them all eventually and I'm open for pull requests.

Examples :

```bash
# Get field by path
$ echo "{foo={bar=<q=e>%true;baz=qqq}}" | ysonq '.foo'
{
    bar=<
        q=e;
    >
    %true;
    baz=qqq;
}

# Get attribute by path
$ echo "{foo={bar=<q=e>%true;baz=qqq}}" | ysonq '.foo.bar.Attrs.q'
e

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
    q=w;
>
e
<
    r=t;
>
y

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
```