#!/usr/bin/env python3
"""Convert a Go coverprofile (coverage.out) to lcov (coverage/lcov.info).

Line-based: each coverprofile block marks lines [startLine,endLine] with its hit
count; a line covered by multiple blocks takes the max. Module-relative paths
(module prefix from go.mod stripped) so repowise maps them to repo files.
"""
import collections
import os

mod = ""
with open("go.mod") as f:
    for line in f:
        if line.startswith("module "):
            mod = line.split()[1].strip()
            break
prefix = mod + "/"

files = collections.OrderedDict()  # relpath -> {line: count}
with open("coverage.out") as f:
    f.readline()  # skip "mode: ..."
    for line in f:
        line = line.strip()
        if not line:
            continue
        loc, _nstmt, count = line.rsplit(" ", 2)
        count = int(count)
        path, rng = loc.split(":")
        rel = path[len(prefix):] if path.startswith(prefix) else path
        start, end = rng.split(",")
        sl = int(start.split(".")[0])
        el = int(end.split(".")[0])
        d = files.setdefault(rel, {})
        for ln in range(sl, el + 1):
            d[ln] = max(d.get(ln, 0), count)

os.makedirs("coverage", exist_ok=True)
out = []
for rel, lines in files.items():
    out.append("SF:" + rel)
    hit = 0
    for ln in sorted(lines):
        c = lines[ln]
        out.append("DA:%d,%d" % (ln, c))
        hit += 1 if c > 0 else 0
    out.append("LF:%d" % len(lines))
    out.append("LH:%d" % hit)
    out.append("end_of_record")
with open("coverage/lcov.info", "w") as f:
    f.write("\n".join(out) + "\n")
print("wrote coverage/lcov.info for %d files" % len(files))
