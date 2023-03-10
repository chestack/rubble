// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.11.4
// source: rpc.proto

package rpc

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// RubbleBackendClient is the client API for RubbleBackend service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type RubbleBackendClient interface {
	AllocateIP(ctx context.Context, in *AllocateIPRequest, opts ...grpc.CallOption) (*AllocateIPReply, error)
	ReleaseIP(ctx context.Context, in *ReleaseIPRequest, opts ...grpc.CallOption) (*ReleaseIPReply, error)
	GetIPInfo(ctx context.Context, in *GetInfoRequest, opts ...grpc.CallOption) (*GetInfoReply, error)
}

type rubbleBackendClient struct {
	cc grpc.ClientConnInterface
}

func NewRubbleBackendClient(cc grpc.ClientConnInterface) RubbleBackendClient {
	return &rubbleBackendClient{cc}
}

func (c *rubbleBackendClient) AllocateIP(ctx context.Context, in *AllocateIPRequest, opts ...grpc.CallOption) (*AllocateIPReply, error) {
	out := new(AllocateIPReply)
	err := c.cc.Invoke(ctx, "/rpc.RubbleBackend/AllocateIP", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *rubbleBackendClient) ReleaseIP(ctx context.Context, in *ReleaseIPRequest, opts ...grpc.CallOption) (*ReleaseIPReply, error) {
	out := new(ReleaseIPReply)
	err := c.cc.Invoke(ctx, "/rpc.RubbleBackend/ReleaseIP", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *rubbleBackendClient) GetIPInfo(ctx context.Context, in *GetInfoRequest, opts ...grpc.CallOption) (*GetInfoReply, error) {
	out := new(GetInfoReply)
	err := c.cc.Invoke(ctx, "/rpc.RubbleBackend/GetIPInfo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// RubbleBackendServer is the server API for RubbleBackend service.
// All implementations must embed UnimplementedRubbleBackendServer
// for forward compatibility
type RubbleBackendServer interface {
	AllocateIP(context.Context, *AllocateIPRequest) (*AllocateIPReply, error)
	ReleaseIP(context.Context, *ReleaseIPRequest) (*ReleaseIPReply, error)
	GetIPInfo(context.Context, *GetInfoRequest) (*GetInfoReply, error)
	mustEmbedUnimplementedRubbleBackendServer()
}

// UnimplementedRubbleBackendServer must be embedded to have forward compatible implementations.
type UnimplementedRubbleBackendServer struct {
}

func (UnimplementedRubbleBackendServer) AllocateIP(context.Context, *AllocateIPRequest) (*AllocateIPReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AllocateIP not implemented")
}
func (UnimplementedRubbleBackendServer) ReleaseIP(context.Context, *ReleaseIPRequest) (*ReleaseIPReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReleaseIP not implemented")
}
func (UnimplementedRubbleBackendServer) GetIPInfo(context.Context, *GetInfoRequest) (*GetInfoReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetIPInfo not implemented")
}
func (UnimplementedRubbleBackendServer) mustEmbedUnimplementedRubbleBackendServer() {}

// UnsafeRubbleBackendServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to RubbleBackendServer will
// result in compilation errors.
type UnsafeRubbleBackendServer interface {
	mustEmbedUnimplementedRubbleBackendServer()
}

func RegisterRubbleBackendServer(s grpc.ServiceRegistrar, srv RubbleBackendServer) {
	s.RegisterService(&RubbleBackend_ServiceDesc, srv)
}

func _RubbleBackend_AllocateIP_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AllocateIPRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RubbleBackendServer).AllocateIP(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.RubbleBackend/AllocateIP",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RubbleBackendServer).AllocateIP(ctx, req.(*AllocateIPRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _RubbleBackend_ReleaseIP_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReleaseIPRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RubbleBackendServer).ReleaseIP(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.RubbleBackend/ReleaseIP",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RubbleBackendServer).ReleaseIP(ctx, req.(*ReleaseIPRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _RubbleBackend_GetIPInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetInfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(RubbleBackendServer).GetIPInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/rpc.RubbleBackend/GetIPInfo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(RubbleBackendServer).GetIPInfo(ctx, req.(*GetInfoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// RubbleBackend_ServiceDesc is the grpc.ServiceDesc for RubbleBackend service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var RubbleBackend_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "rpc.RubbleBackend",
	HandlerType: (*RubbleBackendServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "AllocateIP",
			Handler:    _RubbleBackend_AllocateIP_Handler,
		},
		{
			MethodName: "ReleaseIP",
			Handler:    _RubbleBackend_ReleaseIP_Handler,
		},
		{
			MethodName: "GetIPInfo",
			Handler:    _RubbleBackend_GetIPInfo_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "rpc.proto",
}
