pkgname=hsr
pkgver=1.1.0
pkgrel=1
pkgdesc="Helwan Screen Recorder - X11 Screen and Audio Recorder"
arch=('x86_64')
url="https://github.com/helwan-linux/hel-recorder"
license=('MIT')

depends=(
    'ffmpeg'
    'gtk3'
    'libpulse'
    'libx11'
    'kmag'
    'slop'
    'screenkey'
)

makedepends=(
    'go'
    'git'
    'libx11'
)

source=(
    "$pkgname::git+https://github.com/helwan-linux/hel-recorder.git#branch=main"
)

sha256sums=('SKIP')

prepare() {
    cd "$srcdir/$pkgname/hel-recorder"

    [[ -f go.mod ]] || {
        error "go.mod was not found."
        return 1
    }

    [[ -f assets/icon.png ]] || {
        error "assets/icon.png was not found."
        return 1
    }

    [[ -f style.css ]] || {
        error "style.css was not found."
        return 1
    }

    export CGO_ENABLED=1
    export GOPATH="$srcdir/gopath"
    export GOMODCACHE="$GOPATH/pkg/mod"
    export GOCACHE="$srcdir/gocache"

    go mod download
}

build() {
    cd "$srcdir/$pkgname/hel-recorder"

    export CGO_ENABLED=1
    export GOPATH="$srcdir/gopath"
    export GOMODCACHE="$GOPATH/pkg/mod"
    export GOCACHE="$srcdir/gocache"
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
    cd "$srcdir/$pkgname/hel-recorder"

    export CGO_ENABLED=1
    export GOPATH="$srcdir/gopath"
    export GOMODCACHE="$GOPATH/pkg/mod"
    export GOCACHE="$srcdir/gocache"
    export GOPROXY=off
    export GOSUMDB=off

    go test -mod=readonly ./...
}

package() {
    cd "$srcdir/$pkgname/hel-recorder"

    # Main executable
    install -Dm755 \
        hel-recorder \
        "$pkgdir/usr/bin/hel-recorder"

    # Application icon - 128x128
    install -Dm644 \
        assets/icon.png \
        "$pkgdir/usr/share/icons/hicolor/128x128/apps/hel-recorder.png"

    # Application icon - 256x256
    install -Dm644 \
        assets/icon.png \
        "$pkgdir/usr/share/icons/hicolor/256x256/apps/hel-recorder.png"

    # GTK3 CSS
    install -Dm644 \
        style.css \
        "$pkgdir/usr/share/helwan-screen-recorder/style.css"

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

    # License
    if [[ -f "$srcdir/$pkgname/LICENSE" ]]; then
        install -Dm644 "$srcdir/$pkgname/LICENSE" \
            "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
    fi
}
