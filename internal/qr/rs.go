package qr

// Reed–Solomon error correction over GF(256), with the QR code's
// polynomial x^8 + x^4 + x^3 + x^2 + 1 (0x11D) and generator element 2.

// gfMul multiplies in GF(256).
func gfMul(a, b byte) byte {
	var p byte
	for b != 0 {
		if b&1 != 0 {
			p ^= a
		}
		hi := a&0x80 != 0
		a <<= 1
		if hi {
			a ^= 0x1D
		}
		b >>= 1
	}
	return p
}

// rsGenerator is the generator polynomial of degree n, (x-α^0)…(x-α^(n-1)),
// with its coefficients from the highest power down, leaving out the
// leading 1.
func rsGenerator(n int) []byte {
	g := make([]byte, n)
	g[n-1] = 1 // the polynomial 1
	root := byte(1)
	for range n {
		// Multiply by (x - root).
		for j := range n {
			g[j] = gfMul(g[j], root)
			if j+1 < n {
				g[j] ^= g[j+1]
			}
		}
		root = gfMul(root, 2)
	}
	return g
}

// rsRemainder is the error correction for data: the remainder of
// data·x^n divided by the generator gen of degree n.
func rsRemainder(data, gen []byte) []byte {
	r := make([]byte, len(gen))
	for _, d := range data {
		f := d ^ r[0]
		copy(r, r[1:])
		r[len(r)-1] = 0
		for i, g := range gen {
			r[i] ^= gfMul(g, f)
		}
	}
	return r
}
