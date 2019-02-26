// Code generated by protoc-gen-go. DO NOT EDIT.
// source: pb/optype.proto

package pb

import (
	fmt "fmt"
	math "math"

	proto "github.com/golang/protobuf/proto"
	descriptor "github.com/golang/protobuf/protoc-gen-go/descriptor"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.ProtoPackageIsVersion3 // please upgrade the proto package

type OperationMsg_Operation int32

const (
	OperationMsg_UNKNOWN  OperationMsg_Operation = 0
	OperationMsg_MUTATOR  OperationMsg_Operation = 1
	OperationMsg_ACCESSOR OperationMsg_Operation = 2
)

var OperationMsg_Operation_name = map[int32]string{
	0: "UNKNOWN",
	1: "MUTATOR",
	2: "ACCESSOR",
}

var OperationMsg_Operation_value = map[string]int32{
	"UNKNOWN":  0,
	"MUTATOR":  1,
	"ACCESSOR": 2,
}

func (x OperationMsg_Operation) String() string {
	return proto.EnumName(OperationMsg_Operation_name, int32(x))
}

func (OperationMsg_Operation) EnumDescriptor() ([]byte, []int) {
	return fileDescriptor_4616239e690f8622, []int{0, 0}
}

type OperationMsg struct {
	Op                   OperationMsg_Operation `protobuf:"varint,1,opt,name=op,proto3,enum=praefect.OperationMsg_Operation" json:"op,omitempty"`
	XXX_NoUnkeyedLiteral struct{}               `json:"-"`
	XXX_unrecognized     []byte                 `json:"-"`
	XXX_sizecache        int32                  `json:"-"`
}

func (m *OperationMsg) Reset()         { *m = OperationMsg{} }
func (m *OperationMsg) String() string { return proto.CompactTextString(m) }
func (*OperationMsg) ProtoMessage()    {}
func (*OperationMsg) Descriptor() ([]byte, []int) {
	return fileDescriptor_4616239e690f8622, []int{0}
}

func (m *OperationMsg) XXX_Unmarshal(b []byte) error {
	return xxx_messageInfo_OperationMsg.Unmarshal(m, b)
}
func (m *OperationMsg) XXX_Marshal(b []byte, deterministic bool) ([]byte, error) {
	return xxx_messageInfo_OperationMsg.Marshal(b, m, deterministic)
}
func (m *OperationMsg) XXX_Merge(src proto.Message) {
	xxx_messageInfo_OperationMsg.Merge(m, src)
}
func (m *OperationMsg) XXX_Size() int {
	return xxx_messageInfo_OperationMsg.Size(m)
}
func (m *OperationMsg) XXX_DiscardUnknown() {
	xxx_messageInfo_OperationMsg.DiscardUnknown(m)
}

var xxx_messageInfo_OperationMsg proto.InternalMessageInfo

func (m *OperationMsg) GetOp() OperationMsg_Operation {
	if m != nil {
		return m.Op
	}
	return OperationMsg_UNKNOWN
}

var E_OpType = &proto.ExtensionDesc{
	ExtendedType:  (*descriptor.MessageOptions)(nil),
	ExtensionType: (*OperationMsg)(nil),
	Field:         82302,
	Name:          "praefect.op_type",
	Tag:           "bytes,82302,opt,name=op_type",
	Filename:      "pb/optype.proto",
}

func init() {
	proto.RegisterEnum("praefect.OperationMsg_Operation", OperationMsg_Operation_name, OperationMsg_Operation_value)
	proto.RegisterType((*OperationMsg)(nil), "praefect.OperationMsg")
	proto.RegisterExtension(E_OpType)
}

func init() { proto.RegisterFile("pb/optype.proto", fileDescriptor_4616239e690f8622) }

var fileDescriptor_4616239e690f8622 = []byte{
	// 253 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x6c, 0x8e, 0xb1, 0x4b, 0xc3, 0x40,
	0x14, 0xc6, 0x4d, 0xc0, 0xb6, 0x5e, 0x8b, 0x86, 0x0c, 0x52, 0x5c, 0x0c, 0x9d, 0xba, 0x78, 0xa7,
	0xcd, 0xe6, 0x56, 0x8b, 0x93, 0x24, 0x81, 0x34, 0x45, 0x70, 0x91, 0xbb, 0xf8, 0x7a, 0x04, 0x62,
	0xde, 0xe3, 0xee, 0x3a, 0x64, 0xf5, 0x0f, 0x17, 0x49, 0x43, 0xd4, 0xa1, 0xdb, 0xfb, 0x3e, 0x3e,
	0x7e, 0xef, 0xc7, 0xae, 0x48, 0x09, 0x24, 0xd7, 0x12, 0x70, 0x32, 0xe8, 0x30, 0x9c, 0x90, 0x91,
	0xb0, 0x87, 0xd2, 0xdd, 0x44, 0x1a, 0x51, 0xd7, 0x20, 0x8e, 0xbd, 0x3a, 0xec, 0xc5, 0x07, 0xd8,
	0xd2, 0x54, 0xe4, 0xd0, 0xf4, 0xdb, 0xc5, 0x81, 0xcd, 0x32, 0x02, 0x23, 0x5d, 0x85, 0x4d, 0x62,
	0x75, 0x78, 0xcf, 0x7c, 0xa4, 0xb9, 0x17, 0x79, 0xcb, 0xcb, 0x55, 0xc4, 0x07, 0x10, 0xff, 0xbf,
	0xf9, 0x0b, 0xb9, 0x8f, 0xb4, 0x88, 0xd9, 0xc5, 0x6f, 0x11, 0x4e, 0xd9, 0x78, 0x97, 0xbe, 0xa4,
	0xd9, 0x6b, 0x1a, 0x9c, 0x75, 0x21, 0xd9, 0x15, 0xeb, 0x22, 0xcb, 0x03, 0x2f, 0x9c, 0xb1, 0xc9,
	0x7a, 0xb3, 0x79, 0xde, 0x6e, 0xb3, 0x3c, 0xf0, 0x1f, 0x73, 0x36, 0x46, 0x7a, 0xef, 0x9c, 0xc3,
	0x5b, 0xde, 0x4b, 0xf2, 0x41, 0x92, 0x27, 0x60, 0xad, 0xd4, 0x90, 0x51, 0x87, 0xb4, 0xf3, 0xef,
	0xaf, 0xf3, 0xc8, 0x5b, 0x4e, 0x57, 0xd7, 0xa7, 0x75, 0xf2, 0x11, 0x52, 0xd1, 0x12, 0x3c, 0xc5,
	0x6f, 0x0f, 0xba, 0x72, 0xb5, 0x54, 0xbc, 0xc4, 0x4f, 0xd1, 0x9f, 0x77, 0x68, 0x74, 0x77, 0xca,
	0xba, 0x15, 0x55, 0xe3, 0xc0, 0x34, 0xb2, 0x16, 0x03, 0x46, 0x90, 0x52, 0xa3, 0xe3, 0xd7, 0xf8,
	0x27, 0x00, 0x00, 0xff, 0xff, 0x50, 0xc6, 0x35, 0x37, 0x45, 0x01, 0x00, 0x00,
}