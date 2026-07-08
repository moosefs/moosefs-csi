#!/bin/bash
#
# install-mfsmount.sh -- install the MooseFS client (mfsmount) on the host.
#
# Adds the official MooseFS repository for the detected OS/distribution and
# installs the appropriate client package. Idempotent: re-running on a host
# that already has mfsmount is a no-op (apart from a package-manager refresh).
#
# Designed to be run on a Kubernetes node, typically via a privileged
# DaemonSet that nsenters into the host's mount/PID namespaces (see
# deploy/mfsmount-bootstrap.yaml). This makes "mount -t moosefs" work in the
# host namespace, which is required by the csi-moosefs-node plugin when
# host_namespace_mount=true (so staging mounts survive plugin container
# restarts; see https://github.com/moosefs/moosefs-csi/issues/32).
#
# Usage:
#   sudo ./scripts/install-mfsmount.sh           # direct, on the host
#   kubectl apply -f deploy/mfsmount-bootstrap.yaml  # via DaemonSet on nodes
#
# Exit codes:
#   0  mfsmount installed (or already present)
#   1  unsupported OS / fatal error
#
set -euo pipefail

REPO_KEY_URL="https://repository.moosefs.com/moosefs.key"
REPO_RPM_KEY_URL="https://repository.moosefs.com/RPM-GPG-KEY-MooseFS"
MFS_MAJOR="4"

log()  { printf '[install-mfsmount] %s\n' "$*"; }
warn() { printf '[install-mfsmount] WARNING: %s\n' "$*" >&2; }
die()  { printf '[install-mfsmount] ERROR: %s\n' "$*" >&2; exit 1; }

need_root() {
    [ "$(id -u)" -eq 0 ] || die "must run as root (try: sudo $0)"
}

have() { command -v "$1" >/dev/null 2>&1; }

ensure_curl_gnupg() {
    # Minimal server images (Ubuntu 24.04, Debian cloud, RHEL minimal) may
    # not ship curl/gnupg by default. Install them on demand so repo setup
    # works. Best-effort: if the package manager is unavailable the script
    # will fail later with a clearer error.
    if [ -f /etc/debian_version ]; then
        apt-get update -qq >/dev/null 2>&1 || true
        have curl  || DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends curl
        have gpg   || DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends gnupg
    elif [ -f /etc/redhat-release ]; then
        have dnf && (have curl || dnf install -y curl; have gpg || dnf install -y gnupg2) \
            || (have curl || yum install -y curl; have gpg || yum install -y gnupg2)
    fi
}

have_mfsmount() {
    # /usr/bin/mfsmount is shipped by every MooseFS client package.
    [ -x /usr/bin/mfsmount ] || [ -x /usr/local/bin/mfsmount ] || [ -x /usr/sbin/mfsmount ]
}

detect_arch() {
    case "$(uname -m)" in
        x86_64)  echo amd64 ;;
        aarch64) echo arm64 ;;
        armv7l)  echo armhf ;;
        i386|i686) echo i386 ;;
        *)       uname -m ;;
    esac
}

# ---------------------------------------------------------------------------
# Debian / Ubuntu
# ---------------------------------------------------------------------------
install_debian() {
    local id="$1" codename="$2" arch
    arch="$(detect_arch)"

    log "detected $id $codename ($arch)"

    local keyring_dir="/etc/apt/keyrings"
    local key_path="$keyring_dir/moosefs.gpg"
    mkdir -p "$keyring_dir"

    if [ ! -f "$key_path" ]; then
        log "importing MooseFS GPG key -> $key_path"
        curl -fsSL "$REPO_KEY_URL" | gpg --dearmor -o "$key_path"
        chmod 0644 "$key_path"
    fi

    local repo_file="/etc/apt/sources.list.d/moosefs.list"
    local repo_url="https://repository.moosefs.com/moosefs-${MFS_MAJOR}/apt/$id/$codename"
    log "writing $repo_file"
    echo "deb [arch=$arch signed-by=$key_path] $repo_url $codename main" > "$repo_file"

    log "apt-get update"
    apt-get update

    # moosefs-client on Debian/Ubuntu provides mfsmount + the mount.moosefs helper.
    log "installing moosefs-client"
    DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends moosefs-client

    # Ensure /sbin/mount.moosefs exists for "mount -t moosefs".
    if [ -x /usr/bin/mfsmount ] && [ ! -e /sbin/mount.moosefs ]; then
        ln -s /usr/bin/mfsmount /sbin/mount.moosefs
    fi
}

# ---------------------------------------------------------------------------
# RHEL / CentOS / Rocky / Alma / Fedora
# ---------------------------------------------------------------------------
install_rpm() {
    local id="$1" version_id="$2" el

    case "$version_id" in
        10|16) el=10 ;;
        9|15)  el=9  ;;
        8)     el=8  ;;
        7)     el=7  ;;
        *) die "unsupported RPM distro version: $version_id" ;;
    esac

    log "detected $id $version_id (el$el)"

    local key="/etc/pki/rpm-gpg/RPM-GPG-KEY-MooseFS"
    mkdir -p "$(dirname "$key")"
    if [ ! -f "$key" ]; then
        log "importing MooseFS GPG key -> $key"
        curl -fsSL "$REPO_RPM_KEY_URL" -o "$key"
        rpm --import "$key" 2>/dev/null || true
    fi

    local repo_file="/etc/yum.repos.d/MooseFS.repo"
    log "writing $repo_file"
    cat > "$repo_file" <<EOF
[moosefs-${MFS_MAJOR}]
name=MooseFS ${MFS_MAJOR}
baseurl=https://repository.moosefs.com/moosefs-${MFS_MAJOR}/rpmlfs/el${el}/
enabled=1
gpgcheck=1
gpgkey=file://${key}
EOF

    log "yum/dnf install moosefs-client"
    if have dnf; then
        dnf install -y moosefs-client
    else
        yum install -y moosefs-client
    fi

    if [ -x /usr/bin/mfsmount ] && [ ! -e /sbin/mount.moosefs ]; then
        ln -s /usr/bin/mfsmount /sbin/mount.moosefs
    fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
need_root

if have_mfsmount; then
    log "mfsmount already installed -> nothing to do"
    exit 0
fi

# Install curl/gnupg on minimal images if missing (needed for repo setup).
ensure_curl_gnupg

have curl || die "curl is required (install it first)"
have gpg  || die "gpg is required (install gnupg first)"

if [ ! -f /etc/os-release ]; then
    die "cannot determine OS (/etc/os-release missing)"
fi

. /etc/os-release

case "$ID" in
    ubuntu|debian)
        [ -n "${VERSION_CODENAME:-}" ] || die "VERSION_CODENAME not set in /etc/os-release"
        install_debian "$ID" "$VERSION_CODENAME"
        ;;
    rhel|centos|rocky|alma|fedora)
        install_rpm "$ID" "${VERSION_ID:-}"
        ;;
    sles|opensuse-leap|opensuse*)
        install_rpm "$ID" "${VERSION_ID:-}"
        ;;
    *)
        die "unsupported OS: $ID"
        ;;
esac

have_mfsmount || die "install finished but mfsmount not found on PATH"

log "mfsmount installed: $(mfsmount -V 2>&1 | head -1 || true)"
log "done"