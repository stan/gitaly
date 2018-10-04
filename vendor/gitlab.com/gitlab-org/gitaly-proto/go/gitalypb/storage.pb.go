// Code generated by protoc-gen-go. DO NOT EDIT.
// source: storage.proto

package gitalypb

import proto "github.com/golang/protobuf/proto"
import fmt "fmt"
import math "math"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

type ListDirectoriesRequest struct {
	StorageName string `protobuf:"bytes,1,opt,name=storage_name,json=storageName" json:"storage_name,omitempty"`
	Depth       uint32 `protobuf:"varint,2,opt,name=depth" json:"depth,omitempty"`
}

func (m *ListDirectoriesRequest) Reset()                    { *m = ListDirectoriesRequest{} }
func (m *ListDirectoriesRequest) String() string            { return proto.CompactTextString(m) }
func (*ListDirectoriesRequest) ProtoMessage()               {}
func (*ListDirectoriesRequest) Descriptor() ([]byte, []int) { return fileDescriptor15, []int{0} }

func (m *ListDirectoriesRequest) GetStorageName() string {
	if m != nil {
		return m.StorageName
	}
	return ""
}

func (m *ListDirectoriesRequest) GetDepth() uint32 {
	if m != nil {
		return m.Depth
	}
	return 0
}

type ListDirectoriesResponse struct {
	Paths []string `protobuf:"bytes,1,rep,name=paths" json:"paths,omitempty"`
}

func (m *ListDirectoriesResponse) Reset()                    { *m = ListDirectoriesResponse{} }
func (m *ListDirectoriesResponse) String() string            { return proto.CompactTextString(m) }
func (*ListDirectoriesResponse) ProtoMessage()               {}
func (*ListDirectoriesResponse) Descriptor() ([]byte, []int) { return fileDescriptor15, []int{1} }

func (m *ListDirectoriesResponse) GetPaths() []string {
	if m != nil {
		return m.Paths
	}
	return nil
}

type DeleteAllRepositoriesRequest struct {
	StorageName string `protobuf:"bytes,1,opt,name=storage_name,json=storageName" json:"storage_name,omitempty"`
}

func (m *DeleteAllRepositoriesRequest) Reset()                    { *m = DeleteAllRepositoriesRequest{} }
func (m *DeleteAllRepositoriesRequest) String() string            { return proto.CompactTextString(m) }
func (*DeleteAllRepositoriesRequest) ProtoMessage()               {}
func (*DeleteAllRepositoriesRequest) Descriptor() ([]byte, []int) { return fileDescriptor15, []int{2} }

func (m *DeleteAllRepositoriesRequest) GetStorageName() string {
	if m != nil {
		return m.StorageName
	}
	return ""
}

type DeleteAllRepositoriesResponse struct {
}

func (m *DeleteAllRepositoriesResponse) Reset()                    { *m = DeleteAllRepositoriesResponse{} }
func (m *DeleteAllRepositoriesResponse) String() string            { return proto.CompactTextString(m) }
func (*DeleteAllRepositoriesResponse) ProtoMessage()               {}
func (*DeleteAllRepositoriesResponse) Descriptor() ([]byte, []int) { return fileDescriptor15, []int{3} }

func init() {
	proto.RegisterType((*ListDirectoriesRequest)(nil), "gitaly.ListDirectoriesRequest")
	proto.RegisterType((*ListDirectoriesResponse)(nil), "gitaly.ListDirectoriesResponse")
	proto.RegisterType((*DeleteAllRepositoriesRequest)(nil), "gitaly.DeleteAllRepositoriesRequest")
	proto.RegisterType((*DeleteAllRepositoriesResponse)(nil), "gitaly.DeleteAllRepositoriesResponse")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// Client API for StorageService service

type StorageServiceClient interface {
	ListDirectories(ctx context.Context, in *ListDirectoriesRequest, opts ...grpc.CallOption) (StorageService_ListDirectoriesClient, error)
	DeleteAllRepositories(ctx context.Context, in *DeleteAllRepositoriesRequest, opts ...grpc.CallOption) (*DeleteAllRepositoriesResponse, error)
}

type storageServiceClient struct {
	cc *grpc.ClientConn
}

func NewStorageServiceClient(cc *grpc.ClientConn) StorageServiceClient {
	return &storageServiceClient{cc}
}

func (c *storageServiceClient) ListDirectories(ctx context.Context, in *ListDirectoriesRequest, opts ...grpc.CallOption) (StorageService_ListDirectoriesClient, error) {
	stream, err := grpc.NewClientStream(ctx, &_StorageService_serviceDesc.Streams[0], c.cc, "/gitaly.StorageService/ListDirectories", opts...)
	if err != nil {
		return nil, err
	}
	x := &storageServiceListDirectoriesClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type StorageService_ListDirectoriesClient interface {
	Recv() (*ListDirectoriesResponse, error)
	grpc.ClientStream
}

type storageServiceListDirectoriesClient struct {
	grpc.ClientStream
}

func (x *storageServiceListDirectoriesClient) Recv() (*ListDirectoriesResponse, error) {
	m := new(ListDirectoriesResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *storageServiceClient) DeleteAllRepositories(ctx context.Context, in *DeleteAllRepositoriesRequest, opts ...grpc.CallOption) (*DeleteAllRepositoriesResponse, error) {
	out := new(DeleteAllRepositoriesResponse)
	err := grpc.Invoke(ctx, "/gitaly.StorageService/DeleteAllRepositories", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for StorageService service

type StorageServiceServer interface {
	ListDirectories(*ListDirectoriesRequest, StorageService_ListDirectoriesServer) error
	DeleteAllRepositories(context.Context, *DeleteAllRepositoriesRequest) (*DeleteAllRepositoriesResponse, error)
}

func RegisterStorageServiceServer(s *grpc.Server, srv StorageServiceServer) {
	s.RegisterService(&_StorageService_serviceDesc, srv)
}

func _StorageService_ListDirectories_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(ListDirectoriesRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(StorageServiceServer).ListDirectories(m, &storageServiceListDirectoriesServer{stream})
}

type StorageService_ListDirectoriesServer interface {
	Send(*ListDirectoriesResponse) error
	grpc.ServerStream
}

type storageServiceListDirectoriesServer struct {
	grpc.ServerStream
}

func (x *storageServiceListDirectoriesServer) Send(m *ListDirectoriesResponse) error {
	return x.ServerStream.SendMsg(m)
}

func _StorageService_DeleteAllRepositories_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteAllRepositoriesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StorageServiceServer).DeleteAllRepositories(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gitaly.StorageService/DeleteAllRepositories",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StorageServiceServer).DeleteAllRepositories(ctx, req.(*DeleteAllRepositoriesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _StorageService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "gitaly.StorageService",
	HandlerType: (*StorageServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "DeleteAllRepositories",
			Handler:    _StorageService_DeleteAllRepositories_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "ListDirectories",
			Handler:       _StorageService_ListDirectories_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "storage.proto",
}

func init() { proto.RegisterFile("storage.proto", fileDescriptor15) }

var fileDescriptor15 = []byte{
	// 238 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0xe2, 0xe2, 0x2d, 0x2e, 0xc9, 0x2f,
	0x4a, 0x4c, 0x4f, 0xd5, 0x2b, 0x28, 0xca, 0x2f, 0xc9, 0x17, 0x62, 0x4b, 0xcf, 0x2c, 0x49, 0xcc,
	0xa9, 0x54, 0x0a, 0xe4, 0x12, 0xf3, 0xc9, 0x2c, 0x2e, 0x71, 0xc9, 0x2c, 0x4a, 0x4d, 0x2e, 0xc9,
	0x2f, 0xca, 0x4c, 0x2d, 0x0e, 0x4a, 0x2d, 0x2c, 0x4d, 0x2d, 0x2e, 0x11, 0x52, 0xe4, 0xe2, 0x81,
	0x6a, 0x89, 0xcf, 0x4b, 0xcc, 0x4d, 0x95, 0x60, 0x54, 0x60, 0xd4, 0xe0, 0x0c, 0xe2, 0x86, 0x8a,
	0xf9, 0x25, 0xe6, 0xa6, 0x0a, 0x89, 0x70, 0xb1, 0xa6, 0xa4, 0x16, 0x94, 0x64, 0x48, 0x30, 0x29,
	0x30, 0x6a, 0xf0, 0x06, 0x41, 0x38, 0x4a, 0xfa, 0x5c, 0xe2, 0x18, 0x46, 0x16, 0x17, 0xe4, 0xe7,
	0x15, 0x83, 0x35, 0x14, 0x24, 0x96, 0x64, 0x14, 0x4b, 0x30, 0x2a, 0x30, 0x6b, 0x70, 0x06, 0x41,
	0x38, 0x4a, 0x8e, 0x5c, 0x32, 0x2e, 0xa9, 0x39, 0xa9, 0x25, 0xa9, 0x8e, 0x39, 0x39, 0x41, 0xa9,
	0x05, 0xf9, 0xc5, 0x99, 0xa4, 0xba, 0x44, 0x49, 0x9e, 0x4b, 0x16, 0x87, 0x11, 0x10, 0x9b, 0x8d,
	0x2e, 0x30, 0x72, 0xf1, 0x05, 0x43, 0x34, 0x04, 0xa7, 0x16, 0x95, 0x65, 0x26, 0xa7, 0x0a, 0x85,
	0x71, 0xf1, 0xa3, 0xb9, 0x53, 0x48, 0x4e, 0x0f, 0x12, 0x2c, 0x7a, 0xd8, 0xc3, 0x44, 0x4a, 0x1e,
	0xa7, 0x3c, 0xc4, 0x1a, 0x25, 0x06, 0x03, 0x46, 0xa1, 0x34, 0x2e, 0x51, 0xac, 0x6e, 0x11, 0x52,
	0x81, 0xe9, 0xc6, 0xe7, 0x5b, 0x29, 0x55, 0x02, 0xaa, 0x60, 0x36, 0x25, 0xb1, 0x81, 0x63, 0xd2,
	0x18, 0x10, 0x00, 0x00, 0xff, 0xff, 0xb8, 0xfc, 0x6c, 0x48, 0xda, 0x01, 0x00, 0x00,
}