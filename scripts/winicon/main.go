// Command winicon builds the Windows exe icon from internal/desktop/appicon.png.
//
// The icon is a classic 32-bit bitmap, not a PNG stored inside an ico.
// Explorer paints a PNG icon on white, and the window's LoadIcon cannot
// read one, which shows up as a broken image in the title bar. Resource
// id 3 is the one the window loads.
package main

import (
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"

	"github.com/tc-hib/winres"
)

func main() {
	root := filepath.Join("..", "..")
	if _, err := os.Stat("go.mod"); err != nil {
		root = "."
	}
	in := filepath.Join(root, "internal", "desktop", "appicon.png")
	f, err := os.Open(in)
	if err != nil {
		fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		fatal(err)
	}
	src := toNRGBA(img)

	var rs winres.ResourceSet
	sizes := []int{256, 64, 48, 32, 16}
	group := make([]byte, 6+len(sizes)*14)
	binary.LittleEndian.PutUint16(group[2:], 1)
	binary.LittleEndian.PutUint16(group[4:], uint16(len(sizes)))
	for i, size := range sizes {
		dib := dibIcon(scale(src, size))
		id := uint16(i + 1)
		if err := rs.Set(winres.RT_ICON, winres.ID(id), 0, dib); err != nil {
			fatal(err)
		}
		e := group[6+i*14:]
		if size < 256 {
			e[0] = byte(size)
			e[1] = byte(size)
		}
		binary.LittleEndian.PutUint16(e[4:], 1)  // planes
		binary.LittleEndian.PutUint16(e[6:], 32) // bit count
		binary.LittleEndian.PutUint32(e[8:], uint32(len(dib)))
		binary.LittleEndian.PutUint16(e[12:], id)
	}
	if err := rs.Set(winres.RT_GROUP_ICON, winres.ID(3), 0, group); err != nil {
		fatal(err)
	}
	for _, arch := range []winres.Arch{winres.ArchAMD64, winres.ArchARM64} {
		name := filepath.Join(root, "rsrc_windows_"+string(arch)+".syso")
		out, err := os.Create(name)
		if err != nil {
			fatal(err)
		}
		err = rs.WriteObject(out, arch)
		out.Close()
		if err != nil {
			fatal(err)
		}
	}
}

func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	b := img.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			out.Set(x, y, img.At(x, y))
		}
	}
	return out
}

func scale(src *image.NRGBA, size int) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, size, size))
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	for y := 0; y < size; y++ {
		y0 := y * sh / size
		y1 := (y + 1) * sh / size
		if y1 == y0 {
			y1 = y0 + 1
		}
		for x := 0; x < size; x++ {
			x0 := x * sw / size
			x1 := (x + 1) * sw / size
			if x1 == x0 {
				x1 = x0 + 1
			}
			var r, g, b, a, n int
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					c := src.NRGBAAt(sb.Min.X+xx, sb.Min.Y+yy)
					r += int(c.R)
					g += int(c.G)
					b += int(c.B)
					a += int(c.A)
					n++
				}
			}
			dst.SetNRGBA(x, y, color.NRGBA{uint8(r / n), uint8(g / n), uint8(b / n), uint8(a / n)})
		}
	}
	return dst
}

func dibIcon(img *image.NRGBA) []byte {
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	xorRow := w * 4
	andRow := ((w + 31) / 32) * 4
	xorSize := xorRow * h
	andSize := andRow * h
	buf := make([]byte, 40+xorSize+andSize)
	binary.LittleEndian.PutUint32(buf[0:], 40)
	binary.LittleEndian.PutUint32(buf[4:], uint32(w))
	binary.LittleEndian.PutUint32(buf[8:], uint32(h*2))
	binary.LittleEndian.PutUint16(buf[12:], 1)
	binary.LittleEndian.PutUint16(buf[14:], 32)
	binary.LittleEndian.PutUint32(buf[20:], uint32(xorSize+andSize))
	for y := 0; y < h; y++ {
		sy := h - 1 - y
		for x := 0; x < w; x++ {
			c := img.NRGBAAt(x, sy)
			o := 40 + y*xorRow + x*4
			buf[o] = c.B
			buf[o+1] = c.G
			buf[o+2] = c.R
			buf[o+3] = c.A
			if c.A < 128 {
				buf[40+xorSize+y*andRow+x/8] |= 0x80 >> uint(x%8)
			}
		}
	}
	return buf
}

func fatal(err error) {
	os.Stderr.WriteString(err.Error() + "\n")
	os.Exit(1)
}
