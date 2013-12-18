package main

import (
	"bytes"
	"io"
	"io/ioutil"
)

type FullyBufferedReader struct {
	buffer *[]byte
}

func NewFullyBufferedReader(src io.Reader) *FullyBufferedReader {
	data, err := ioutil.ReadAll(src)
	if err != nil {
		return nil
	} else {
		return &FullyBufferedReader{
			buffer: &data,
		}
	}
}

func (f *FullyBufferedReader) Reader() *FullyBufferedReadCloser {
	return &FullyBufferedReadCloser{
		DummyReadCloser: DummyReadCloser{bytes.NewReader(*f.buffer)},
	}
}

type DummyReadCloser struct {
	reader io.Reader
}

func NewDummyReadCloser(reader io.Reader) DummyReadCloser {
	return DummyReadCloser{reader: reader}
}

func (f *DummyReadCloser) Read(buf []byte) (int, error) {
	return f.reader.Read(buf)
}

func (f *DummyReadCloser) Close() error {
	return nil
}

type FullyBufferedReadCloser struct {
	DummyReadCloser
}
