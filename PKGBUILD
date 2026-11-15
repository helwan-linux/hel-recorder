
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
    'xzoom'
)

makedepends=(
    'go'
    'git'
)

source=(
    "$pkgname::git+https://github.com/helwan-linux/hel-recorder.git#branch=main"
)

sha256sums=('SKIP')

prepare() {
    cd "$srcdir/$pkgname"

    if [[ ! -f go.mod ]]; then
        error "go.mod was not found in the project root."
        return 1
    fi

    go mod download
}

build() {
    cd "$srcdir/$pkgname"

    export CGO_ENABLED=1

    go build \
        -mod=readonly \
        -trimpath \
        -buildvcs=false \
        -o hel-recorder \
        .
}

check() {
    cd "$srcdir/$pkgname"
    go test -mod=readonly ./...
}

package() {
    cd "$srcdir/$pkgname"

    install -Dm755 \
        hel-recorder \
        "$pkgdir/usr/bin/hel-recorder"

    install -Dm644 \
        /dev/stdin \
        "$pkgdir/usr/share/applications/helwan-screen-recorder.desktop" <<'EOF'
[Desktop Entry]
Name=Helwan Screen Recorder
GenericName=Screen Recorder
Comment=Record Screen and Audio
Exec=hel-recorder
Icon=video-record
Terminal=false
Type=Application
Categories=AudioVideo;Video;Recorder;
StartupNotify=true
EOF
}
