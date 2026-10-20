package parser

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

const DbHeaderSizeBytes = 100

const minUsablePageSize = 480

const (
	dbMagicLenBytes                  = 16
	dbPageSizeLenBytes               = 2
	dbFileFormatWriteVersionLenBytes = 1 // 1 for legacy; 2 for WAL.
	dbFileFormatReadVersionLenBytes  = 1 // 1 for legacy; 2 for WAL.
	dbReservedBytesLenBytes          = 1
)

type DbHeader struct {
	MagicHeaderString      string
	PageSize               uint32
	FileFormatWriteVersion byte
	FileFormatReadVersion  byte
	ReservedBytes          uint8
}

func (h *DbHeader) Parse(dbf *os.File) error {
	buf := make([]byte, DbHeaderSizeBytes)

	if _, err := io.ReadFull(dbf, buf); err != nil {
		return err
	}

	off := 0

	h.MagicHeaderString = string(buf[off : off+dbMagicLenBytes])
	off += dbMagicLenBytes

	h.PageSize = uint32(binary.BigEndian.Uint16(buf[off : off+dbPageSizeLenBytes]))
	if h.PageSize == 1 {
		h.PageSize = 65536
	}
	if h.PageSize < 512 || h.PageSize > 65536 ||
		h.PageSize&(h.PageSize-1) != 0 {
		return fmt.Errorf("invalid database page size: %d", h.PageSize)
	}
	off += dbPageSizeLenBytes

	h.FileFormatWriteVersion = buf[off]
	off += dbFileFormatWriteVersionLenBytes

	h.FileFormatReadVersion = buf[off]
	off += dbFileFormatReadVersionLenBytes

	h.ReservedBytes = buf[off]
	usableSize := h.PageSize - uint32(h.ReservedBytes)
	if usableSize < minUsablePageSize {
		return fmt.Errorf("invalid usable page size: %d", usableSize)
	}
	off += dbReservedBytesLenBytes

	return nil
}
