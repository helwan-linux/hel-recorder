
pkgname=hsr
pkgver=1.0.0
pkgrel=4
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

    install -Dm755 \
        hel-recorder \
        "$pkgdir/usr/bin/hel-recorder"

    install -Dm644 \
        assets/icon.png \
        "$pkgdir/usr/share/icons/hicolor/256x256/apps/hel-recorder.png"

    install -Dm644 \
        assets/icon.png \
        "$pkgdir/usr/share/hel-recorder/assets/icon.png"

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

    if [[ -f "$srcdir/$pkgname/LICENSE" ]]; then
        install -Dm644 "$srcdir/$pkgname/LICENSE" \
            "$pkgdir/usr/share/licenses/$pkgname/LICENSE"
    fi
}

