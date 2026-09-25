package hid

// ParseUsage returns the first Usage Page and the Usage that follows it in a HID
// report descriptor. Linux gives us the raw descriptor; Windows computes this itself.
func ParseUsage(desc []byte) (page, usage uint16) {
	havePage := false
	for i := 0; i < len(desc); {
		prefix := desc[i]
		if prefix == 0xFE { // long item: FE, size, tag, data...
			if i+1 >= len(desc) {
				break
			}
			i += 3 + int(desc[i+1])
			continue
		}
		size := int(prefix & 0x03)
		if size == 3 {
			size = 4
		}
		if i+1+size > len(desc) {
			break
		}
		var value uint32
		for b := 0; b < size; b++ {
			value |= uint32(desc[i+1+b]) << (8 * b)
		}
		itemType, tag := (prefix>>2)&0x03, prefix>>4
		switch {
		case itemType == 1 && tag == 0 && !havePage: // Global: Usage Page
			page, havePage = uint16(value), true
		case itemType == 2 && tag == 0 && havePage: // Local: Usage
			return page, uint16(value)
		}
		i += 1 + size
	}
	return page, 0
}
