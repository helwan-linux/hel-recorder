
pkgname=hsr
pkgver=0.1.0
pkgrel=1
pkgdesc="Helwan Screen Recorder - X11 Screen and Audio Recorder"
arch=('x86_64')
url="https://github.com/helwan-linux/hel-recorder"
license=('MIT')

depends=(
    'ffmpeg'
    'gtk3'
    'libpulse'
    'kmag'
)

makedepends=(
    'go'
    'git'
)

source=(
    "$pkgname::git+https://github.com/helwan-linux/hel-recorder.git#branch=main"
)

sha256sums=('SKIP')

export CGO_ENABLED=1
export GOPATH="$srcdir/gopath"
export GOMODCACHE="$GOPATH/pkg/mod"
export GOCACHE="$srcdir/gocache"

pkgver() {
    cd "$srcdir/$pkgname"

    printf "0.1.0.r%s.%s" \
        "$(git rev-list --count HEAD)" \
        "$(git rev-parse --short HEAD)"
}

prepare() {
    cd "$srcdir/$pkgname"

    [[ -f go.mod ]] || {
        error "go.mod was not found."
        return 1
    }

    [[ -f LICENSE ]] || {
        error "LICENSE file was not found."
        return 1
    }

    [[ -f assets/icon.png ]] || {
        error "assets/icon.png was not found."
        return 1
    }

    go mod download
}

build() {
    cd "$srcdir/$pkgname"

    export GOPROXY=off
    export GOSUMDB=off

    go build \
        -mod=readonly \
        -trimpath \
        -buildvcs=false \
        -o hel-recorder \
        .
}

check() {
    cd "$srcdir/$pkgname"

    export GOPROXY=off
    export GOSUMDB=off

    go test -mod=readonly ./...
}

package() {
    cd "$srcdir/$pkgname"

    # Main executable
    install -Dm755 \
        hel-recorder \
        "$pkgdir/usr/bin/hel-recorder"

    # Application icon
    install -Dm644 \
        assets/icon.png \
        "$pkgdir/usr/share/icons/hicolor/256x256/apps/hel-recorder.png"

    # Application asset
    install -Dm644 \
        assets/icon.png \
        "$pkgdir/usr/share/hel-recorder/assets/icon.png"

    # License
    install -Dm644 \
        LICENSE \
        "$pkgdir/usr/share/licenses/$pkgname/LICENSE"

    # Desktop entry
    install -Dm644 /dev/stdin \
        "$pkgdir/usr/share/applications/helwan-screen-recorder.desktop" <<'EOF'
[Desktop Entry]
Name=Helwan Screen Recorder
GenericName=Screen Recorder
Comment=Record Screen and Audio
Exec=hel-recorder
Icon=hel-recorder
Terminal=false
Type=Application
Categories=AudioVideo;Video;Recorder;
StartupNotify=true
StartupWMClass=hel-recorder
EOF

    # Documentation
    if [[ -f README.md ]]; then
        install -Dm644 \
            README.md \
            "$pkgdir/usr/share/doc/$pkgname/README.md"
    fi
}

