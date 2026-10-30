package cas

import (
	"strings"
	"unicode/utf16"
)

// 上游登录页 des.js 自带一份非标准 DES（密钥编排按 PC1 表的转置取位），
// 服务端与之匹配。这里不做等价化简，逐位复刻，任何"修正"都会导致登录失败。

type bits64 [64]byte

func strEnc(data string) string {
	units := utf16.Encode([]rune(data))

	var b strings.Builder
	b.Grow((len(units) + 3) / 4 * 16)

	keys := [3][][64]byte{getKeyBytes("1"), getKeyBytes("2"), getKeyBytes("3")}
	for i := 0; i < len(units); i += 4 {
		end := i + 4
		if end > len(units) {
			end = len(units)
		}
		bt := strToBt(units[i:end])
		for _, ks := range keys {
			for _, k := range ks {
				bt = enc(bt, k)
			}
		}
		b.WriteString(bt64ToHex(bt))
	}

	return b.String()
}

func getKeyBytes(key string) [][64]byte {
	units := utf16.Encode([]rune(key))
	out := make([][64]byte, 0, (len(units)+3)/4)
	for i := 0; i < len(units); i += 4 {
		out = append(out, strToBt(units[i:min(i+4, len(units))]))
	}
	return out
}

func strToBt(units []uint16) bits64 {
	var bt bits64
	for i := 0; i < len(units) && i < 4; i++ {
		for j := 0; j < 16; j++ {
			bt[16*i+j] = byte(units[i]>>(15-j)) & 1
		}
	}
	return bt
}

func bt64ToHex(bt bits64) string {
	const digits = "0123456789ABCDEF"
	out := make([]byte, 16)
	for i := 0; i < 16; i++ {
		v := bt[i*4]*8 + bt[i*4+1]*4 + bt[i*4+2]*2 + bt[i*4+3]
		out[i] = digits[v]
	}
	return string(out)
}

func enc(data, keyByte bits64) bits64 {
	keys := generateKeys(keyByte)
	ip := initPermute(data)

	var left, right [32]byte
	copy(left[:], ip[:32])
	copy(right[:], ip[32:])

	for i := 0; i < 16; i++ {
		prev := left
		left = right
		right = xor32(pPermute(sBoxPermute(xor48(expandPermute(right), keys[i]))), prev)
	}

	var out bits64
	copy(out[:32], right[:])
	copy(out[32:], left[:])
	return finallyPermute(out)
}

func generateKeys(keyByte bits64) [16][48]byte {
	var key [56]byte
	for i := 0; i < 7; i++ {
		for j, k := 0, 7; j < 8; j, k = j+1, k-1 {
			key[i*8+j] = keyByte[8*k+i]
		}
	}

	loop := [16]int{1, 1, 2, 2, 2, 2, 2, 2, 1, 2, 2, 2, 2, 2, 2, 1}
	selectTable := [48]int{
		13, 16, 10, 23, 0, 4, 2, 27, 14, 5, 20, 9, 22, 18, 11, 3,
		25, 7, 15, 6, 26, 19, 12, 1, 40, 51, 30, 36, 46, 54, 29, 39,
		50, 44, 32, 47, 43, 48, 38, 55, 33, 52, 45, 41, 49, 35, 28, 31,
	}

	var keys [16][48]byte
	for i := 0; i < 16; i++ {
		for s := 0; s < loop[i]; s++ {
			head, mid := key[0], key[28]
			for k := 0; k < 27; k++ {
				key[k] = key[k+1]
				key[28+k] = key[29+k]
			}
			key[27] = head
			key[55] = mid
		}
		for m := 0; m < 48; m++ {
			keys[i][m] = key[selectTable[m]]
		}
	}
	return keys
}

func initPermute(in bits64) bits64 {
	var out bits64
	for i, m, n := 0, 1, 0; i < 4; i, m, n = i+1, m+2, n+2 {
		for j, k := 7, 0; j >= 0; j, k = j-1, k+1 {
			out[i*8+k] = in[j*8+m]
			out[i*8+k+32] = in[j*8+n]
		}
	}
	return out
}

func expandPermute(right [32]byte) [48]byte {
	var out [48]byte
	for i := 0; i < 8; i++ {
		if i == 0 {
			out[0] = right[31]
		} else {
			out[i*6] = right[i*4-1]
		}
		out[i*6+1] = right[i*4]
		out[i*6+2] = right[i*4+1]
		out[i*6+3] = right[i*4+2]
		out[i*6+4] = right[i*4+3]
		if i == 7 {
			out[47] = right[0]
		} else {
			out[i*6+5] = right[i*4+4]
		}
	}
	return out
}

func xor32(a, b [32]byte) [32]byte {
	var out [32]byte
	for i := range out {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func xor48(a, b [48]byte) [48]byte {
	var out [48]byte
	for i := range out {
		out[i] = a[i] ^ b[i]
	}
	return out
}

func sBoxPermute(expand [48]byte) [32]byte {
	var out [32]byte
	for m := 0; m < 8; m++ {
		row := int(expand[m*6+0])*2 + int(expand[m*6+5])
		col := int(expand[m*6+1])*8 + int(expand[m*6+2])*4 + int(expand[m*6+3])*2 + int(expand[m*6+4])
		v := sBoxes[m][row][col]
		out[m*4] = v >> 3 & 1
		out[m*4+1] = v >> 2 & 1
		out[m*4+2] = v >> 1 & 1
		out[m*4+3] = v & 1
	}
	return out
}

func pPermute(in [32]byte) [32]byte {
	table := [32]int{
		15, 6, 19, 20, 28, 11, 27, 16, 0, 14, 22, 25, 4, 17, 30, 9,
		1, 7, 23, 13, 31, 26, 2, 8, 18, 12, 29, 5, 21, 10, 3, 24,
	}
	var out [32]byte
	for i, idx := range table {
		out[i] = in[idx]
	}
	return out
}

func finallyPermute(in bits64) bits64 {
	table := [64]int{
		39, 7, 47, 15, 55, 23, 63, 31, 38, 6, 46, 14, 54, 22, 62, 30,
		37, 5, 45, 13, 53, 21, 61, 29, 36, 4, 44, 12, 52, 20, 60, 28,
		35, 3, 43, 11, 51, 19, 59, 27, 34, 2, 42, 10, 50, 18, 58, 26,
		33, 1, 41, 9, 49, 17, 57, 25, 32, 0, 40, 8, 48, 16, 56, 24,
	}
	var out bits64
	for i, idx := range table {
		out[i] = in[idx]
	}
	return out
}

var sBoxes = [8][4][16]byte{
	{
		{14, 4, 13, 1, 2, 15, 11, 8, 3, 10, 6, 12, 5, 9, 0, 7},
		{0, 15, 7, 4, 14, 2, 13, 1, 10, 6, 12, 11, 9, 5, 3, 8},
		{4, 1, 14, 8, 13, 6, 2, 11, 15, 12, 9, 7, 3, 10, 5, 0},
		{15, 12, 8, 2, 4, 9, 1, 7, 5, 11, 3, 14, 10, 0, 6, 13},
	},
	{
		{15, 1, 8, 14, 6, 11, 3, 4, 9, 7, 2, 13, 12, 0, 5, 10},
		{3, 13, 4, 7, 15, 2, 8, 14, 12, 0, 1, 10, 6, 9, 11, 5},
		{0, 14, 7, 11, 10, 4, 13, 1, 5, 8, 12, 6, 9, 3, 2, 15},
		{13, 8, 10, 1, 3, 15, 4, 2, 11, 6, 7, 12, 0, 5, 14, 9},
	},
	{
		{10, 0, 9, 14, 6, 3, 15, 5, 1, 13, 12, 7, 11, 4, 2, 8},
		{13, 7, 0, 9, 3, 4, 6, 10, 2, 8, 5, 14, 12, 11, 15, 1},
		{13, 6, 4, 9, 8, 15, 3, 0, 11, 1, 2, 12, 5, 10, 14, 7},
		{1, 10, 13, 0, 6, 9, 8, 7, 4, 15, 14, 3, 11, 5, 2, 12},
	},
	{
		{7, 13, 14, 3, 0, 6, 9, 10, 1, 2, 8, 5, 11, 12, 4, 15},
		{13, 8, 11, 5, 6, 15, 0, 3, 4, 7, 2, 12, 1, 10, 14, 9},
		{10, 6, 9, 0, 12, 11, 7, 13, 15, 1, 3, 14, 5, 2, 8, 4},
		{3, 15, 0, 6, 10, 1, 13, 8, 9, 4, 5, 11, 12, 7, 2, 14},
	},
	{
		{2, 12, 4, 1, 7, 10, 11, 6, 8, 5, 3, 15, 13, 0, 14, 9},
		{14, 11, 2, 12, 4, 7, 13, 1, 5, 0, 15, 10, 3, 9, 8, 6},
		{4, 2, 1, 11, 10, 13, 7, 8, 15, 9, 12, 5, 6, 3, 0, 14},
		{11, 8, 12, 7, 1, 14, 2, 13, 6, 15, 0, 9, 10, 4, 5, 3},
	},
	{
		{12, 1, 10, 15, 9, 2, 6, 8, 0, 13, 3, 4, 14, 7, 5, 11},
		{10, 15, 4, 2, 7, 12, 9, 5, 6, 1, 13, 14, 0, 11, 3, 8},
		{9, 14, 15, 5, 2, 8, 12, 3, 7, 0, 4, 10, 1, 13, 11, 6},
		{4, 3, 2, 12, 9, 5, 15, 10, 11, 14, 1, 7, 6, 0, 8, 13},
	},
	{
		{4, 11, 2, 14, 15, 0, 8, 13, 3, 12, 9, 7, 5, 10, 6, 1},
		{13, 0, 11, 7, 4, 9, 1, 10, 14, 3, 5, 12, 2, 15, 8, 6},
		{1, 4, 11, 13, 12, 3, 7, 14, 10, 15, 6, 8, 0, 5, 9, 2},
		{6, 11, 13, 8, 1, 4, 10, 7, 9, 5, 0, 15, 14, 2, 3, 12},
	},
	{
		{13, 2, 8, 4, 6, 15, 11, 1, 10, 9, 3, 14, 5, 0, 12, 7},
		{1, 15, 13, 8, 10, 3, 7, 4, 12, 5, 6, 11, 0, 14, 9, 2},
		{7, 11, 4, 1, 9, 12, 14, 2, 0, 6, 10, 13, 15, 3, 5, 8},
		{2, 1, 14, 7, 4, 10, 8, 13, 15, 12, 9, 0, 3, 5, 6, 11},
	},
}
