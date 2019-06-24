package flvcodec

import (
	"os"
	"fmt"
	"encoding/binary"
	"go_srs/srs/codec"
	"go_srs/srs/protocol/rtmp"
	"go_srs/srs/utils"
)

func VideoIsKeyFrame(data []byte) bool {
	if len(data) < 1 {
		return false
	}

	frameType := (data[0] >> 4) & 0x0F
	return frameType == codec.SrsCodecVideoAVCFrameKeyFrame
}

func VideoIsSequenceHeader(data []byte) bool {
	if !VideoIsH264(data) {
		return false
	}

	if len(data) < 2 {
		return false
	}

	formatType := (data[0] >> 4) & 0x0F
	avcPacketType := data[1]
	return formatType == codec.SrsCodecVideoAVCFrameKeyFrame && avcPacketType == codec.SrsCodecVideoAVCTypeSequenceHeader
}

func AudioIsSequenceHeader(data []byte) bool {
	if !AudioIsAAC(data) {
		return false
	}

	if len(data) < 2 {
		return false
	}

	aacPacketType := data[1]
	return aacPacketType == codec.SrsCodecAudioTypeSequenceHeader
}

func VideoIsH264(data []byte) bool {
	if len(data) < 1 {
		return false
	}

	codecId := data[0] & 0x0F
	return codecId == codec.SrsCodecVideoAVC
}

func AudioIsAAC(data []byte) bool {
	if len(data) < 1 {
		return false
	}

	soundFormat := (data[0] >> 4) & 0x0F
	return soundFormat == codec.SrsCodecAudioAAC
}

func VideoIsAcceptable(data []byte) bool {
	if len(data) < 1 {
		return false
	}

	formatType := data[0]
	codecId := formatType & 0x0F
	formatType = (formatType >> 4) & 0x0F

	if formatType < 1 || formatType > 5 {
		return false
	}

	if codecId < 2 || codecId > 7 {
		return false
	}

	return true
}


const (
	AudioTagType	=	0x08
	VideoTagType	= 	0x09
	MetaDataTagType	= 	0x18
)

const (
	SRS_FLV_TAG_HEADER_SIZE = 11
	SRS_FLV_PREVIOUS_TAG_SIZE = 4
)

type SrsFlvHeader struct {
	signature 	[]byte  //FLV
	version		byte
	flags		byte	//第0位和第2位,分别表示 audio 与 video 存在的情况.(1表示存在,0表示不存在)。
	headerSize	[]byte	//即自身的总长度，一直为9, 4字节
}

func NewSrsFlvHeader(hasAudio bool, hasVideo bool) *SrsFlvHeader {
	var f byte = 0
	if hasAudio {
		f |= 1 << 0
	}

	if hasVideo {
		f |= 1 << 2
	}

	h := utils.Int32ToBytes(0x09, binary.BigEndian)
	return &SrsFlvHeader{
		signature:[]byte{'F','L','V'},
		version:0x01,
		flags:f,
		headerSize:h,
	}
}

func (this *SrsFlvHeader) Data() []byte {
	data := make([]byte, 0)
	data = append(data, this.signature...)
	data = append(data, this.version)
	data = append(data, this.flags)
	data = append(data, this.headerSize...)
	return data
}

type TagHeader struct {
	tagType		byte
	dataSize	[]byte //3byte
	timestamp	[]byte	//4byte
	reserved	[]byte	//全0
}

func NewTagHeader(typ byte, timestamp uint32, dataSize int32) *TagHeader {
	s := utils.Int32ToBytes(dataSize, binary.BigEndian)
	t := utils.UInt32ToBytes(timestamp, binary.BigEndian)
	return &TagHeader{
		tagType:typ, 
		dataSize:s[1:4],
		timestamp:t,
		reserved:[]byte{0,0,0},
	}
}

func (this *TagHeader) Data() []byte {
	data := make([]byte, 0)
	data = append(data, this.tagType)
	data = append(data, this.dataSize...)
	data = append(data, this.timestamp...)
	data = append(data, this.reserved...)
	return data
}

type SrsFlvEncoder struct {
	header 	*SrsFlvHeader
	file 	*os.File
}

func NewSrsFlvEncoder(f *os.File) *SrsFlvEncoder {
	return &SrsFlvEncoder{
		file:f,
		header:NewSrsFlvHeader(true, true),
	}
}

func (this *SrsFlvEncoder) WriteHeader() error {
	// 9bytes header and 4bytes first previous-tag-size
	if _, err := this.file.Write(this.header.Data()); err != nil {
		return err
	}
	// previous tag size.
	pts := []byte{0x00, 0x00, 0x00, 0x00}
	if _, err := this.file.Write(pts); err != nil {
		return err
	}
	return nil
}

func (this *SrsFlvEncoder) WriteMetaData(data []byte) (uint32, error) {
	fmt.Println("**************WriteMetaData******************, len=", len(data))
	header := NewTagHeader(MetaDataTagType, 0, int32(len(data)))
	return this.writeTag(header, data)
}

func (this *SrsFlvEncoder) WriteAudio(timestamp uint32, data []byte) (uint32, error) {
	// fmt.Println("**************WriteAudio******************")
	header := NewTagHeader(AudioTagType, timestamp, int32(len(data)))
	return this.writeTag(header, data)
}

func (this *SrsFlvEncoder) WriteVideo(timestamp uint32, data []byte) (uint32, error) {
	// fmt.Println("**************WriteVideo******************")
	header := NewTagHeader(VideoTagType, timestamp, int32(len(data)))
	return this.writeTag(header, data)
}

func (this *SrsFlvEncoder) writeTag(header *TagHeader, data []byte) (uint32, error) {
	d := header.Data()
	d = append(d, data...)

	prevTagSize := int32(len(d))
	p := utils.Int32ToBytes(prevTagSize, binary.BigEndian)
	d = append(d, p...)
	n, err := this.file.Write(d)
	_ = n
	return uint32(len(d)), err
}

func (this *SrsFlvEncoder) WriteTags(msgs []*rtmp.SrsRtmpMessage) error {
	for i := 0; i < len(msgs); i++ {
		if msgs[i].GetHeader().IsAudio() {
			_, _ = this.WriteAudio(uint32(msgs[i].GetHeader().GetTimestamp()), msgs[i].GetPayload())
		} else if(msgs[i].GetHeader().IsVideo()) {
			_, _ = this.WriteVideo(uint32(msgs[i].GetHeader().GetTimestamp()), msgs[i].GetPayload())
		} else {
			_, _ = this.WriteMetaData(msgs[i].GetPayload())
		}
	}
	return nil
}
