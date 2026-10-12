package parser

import (
	"encoding/binary"
	"io"
	"os"
)

const DbHeaderSizeBytes = 100

const (
	dbMagicOffset = 0
	dbMagicLength = 16

	dbPageSizeOffset = 16
	dbPageSizeLength = 2
)

type DbHeader struct {
	MagicHeaderString string
	PageSize          uint32
}

func (h *DbHeader) Parse(dbf *os.File) error {
	buf := make([]byte, DbHeaderSizeBytes)

	if _, err := io.ReadFull(dbf, buf); err != nil {
		return err
	}

	h.MagicHeaderString = string(buf[dbMagicOffset : dbMagicOffset+dbMagicLength])
	h.PageSize = uint32(binary.BigEndian.Uint16(buf[dbPageSizeOffset : dbPageSizeOffset+dbPageSizeLength]))
	if h.PageSize == 1 {
		h.PageSize = 65536
	}

	return nil
}
