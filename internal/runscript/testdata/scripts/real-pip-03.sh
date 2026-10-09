dnf update -y
# copr-cli comes from Fedora's own packages — Fedora 38+
# enforces PEP 668 and blocks `pip install` into the system
# Python. `gh` is needed by the validate-inputs step below; it
# runs inside this `fedora:45` container, where the host
# runner's pre-installed `gh` isn't visible.
dnf install -y rpm-build rpmdevtools copr-cli git rust cargo gcc openssl-devel pkgconf-pkg-config tar gzip gh
rpmdev-setuptree
