package maplibre

/*
#include "maplibre_native_c.h"

static inline mln_opengl_context_descriptor mln_go_opengl_context_descriptor_wgl(
    mln_wgl_context_descriptor wgl) {
  mln_opengl_context_descriptor out;
  out.size = sizeof(mln_opengl_context_descriptor);
  out.platform = MLN_OPENGL_CONTEXT_PLATFORM_WGL;
  out.data.wgl = wgl;
  return out;
}

static inline mln_opengl_context_descriptor mln_go_opengl_context_descriptor_egl(
    mln_egl_context_descriptor egl) {
  mln_opengl_context_descriptor out;
  out.size = sizeof(mln_opengl_context_descriptor);
  out.platform = MLN_OPENGL_CONTEXT_PLATFORM_EGL;
  out.data.egl = egl;
  return out;
}
*/
import "C"

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/maplibre/maplibre-native-ffi/bindings/go/internal/handle"
	internalstatus "github.com/maplibre/maplibre-native-ffi/bindings/go/internal/status"
)

// RenderBackendMask preserves the render backend bits reported by the native
// library. Unknown future bits remain in the mask.
type RenderBackendMask uint32

const (
	RenderBackendMetal  RenderBackendMask = RenderBackendMask(C.MLN_RENDER_BACKEND_FLAG_METAL)
	RenderBackendOpenGL RenderBackendMask = RenderBackendMask(C.MLN_RENDER_BACKEND_FLAG_OPENGL)
	RenderBackendVulkan RenderBackendMask = RenderBackendMask(C.MLN_RENDER_BACKEND_FLAG_VULKAN)
)

// Has reports whether all backend bits in backend are present.
func (mask RenderBackendMask) Has(backend RenderBackendMask) bool {
	return mask&backend == backend
}

// OpenGLContextProviderMask preserves the OpenGL context provider bits
// reported by the native library. Unknown future bits remain in the mask.
type OpenGLContextProviderMask uint32

const (
	OpenGLContextProviderWGL OpenGLContextProviderMask = OpenGLContextProviderMask(C.MLN_OPENGL_CONTEXT_PROVIDER_FLAG_WGL)
	OpenGLContextProviderEGL OpenGLContextProviderMask = OpenGLContextProviderMask(C.MLN_OPENGL_CONTEXT_PROVIDER_FLAG_EGL)
)

// Has reports whether all provider bits in provider are present.
func (mask OpenGLContextProviderMask) Has(provider OpenGLContextProviderMask) bool {
	return mask&provider == provider
}

// OpenGLContextPlatform selects the OpenGL context provider variant inside an
// OpenGLContextDescriptor. Use NewOpenGLContextWGL or NewOpenGLContextEGL to
// construct descriptors with the platform set.
type OpenGLContextPlatform uint32

const (
	OpenGLContextPlatformUnspecified OpenGLContextPlatform = OpenGLContextPlatform(C.MLN_OPENGL_CONTEXT_PLATFORM_UNSPECIFIED)
	OpenGLContextPlatformWGL         OpenGLContextPlatform = OpenGLContextPlatform(C.MLN_OPENGL_CONTEXT_PLATFORM_WGL)
	OpenGLContextPlatformEGL         OpenGLContextPlatform = OpenGLContextPlatform(C.MLN_OPENGL_CONTEXT_PLATFORM_EGL)
)

// NativePointer is a borrowed opaque backend-native address.
//
// It grants no memory access and transfers no ownership. The binding converts
// it to unsafe.Pointer only at cgo boundaries for C APIs that accept opaque
// backend handles.
type NativePointer uintptr

// RenderTargetExtent is a logical render target extent in UI pixels.
type RenderTargetExtent struct {
	Width       uint32
	Height      uint32
	ScaleFactor float64
}

// TextureImageInfo describes CPU readback image metadata.
type TextureImageInfo struct {
	Width      uint32
	Height     uint32
	Stride     uint32
	ByteLength uint64
}

// MetalContextDescriptor contains Metal backend context handles.
type MetalContextDescriptor struct {
	Device NativePointer
}

// VulkanContextDescriptor contains Vulkan backend context handles.
type VulkanContextDescriptor struct {
	Instance                 NativePointer
	PhysicalDevice           NativePointer
	Device                   NativePointer
	GraphicsQueue            NativePointer
	GraphicsQueueFamilyIndex uint32
}

// MetalSurfaceDescriptor describes a Metal-backed surface render target. The
// session retains the layer and optional device while attached.
type MetalSurfaceDescriptor struct {
	Extent  RenderTargetExtent
	Context MetalContextDescriptor
	Layer   NativePointer
}

// VulkanSurfaceDescriptor describes a Vulkan-backed surface render target.
// Vulkan handles are borrowed and must remain valid until detach or session
// close. The device must support swapchain presentation on the graphics queue
// family for Surface.
type VulkanSurfaceDescriptor struct {
	Extent  RenderTargetExtent
	Context VulkanContextDescriptor
	Surface NativePointer
}

// MetalOwnedTextureDescriptor describes a Metal session-owned texture render target.
type MetalOwnedTextureDescriptor struct {
	Extent  RenderTargetExtent
	Context MetalContextDescriptor
}

// MetalBorrowedTextureDescriptor describes a Metal caller-owned texture render
// target. The caller keeps Texture valid until detach or session close and
// synchronizes all use outside this session.
type MetalBorrowedTextureDescriptor struct {
	Extent  RenderTargetExtent
	Texture NativePointer
}

// VulkanOwnedTextureDescriptor describes a Vulkan session-owned texture render
// target. Vulkan context handles are borrowed and must remain valid until detach
// or session close.
type VulkanOwnedTextureDescriptor struct {
	Extent  RenderTargetExtent
	Context VulkanContextDescriptor
}

// VulkanBorrowedTextureDescriptor describes a Vulkan caller-owned texture render
// target. The caller keeps Image and ImageView valid until detach or session
// close, manages queue-family ownership, makes the image available in
// InitialLayout before each RenderUpdate, avoids concurrent use during the
// update, and observes FinalLayout after RenderUpdate returns.
type VulkanBorrowedTextureDescriptor struct {
	Extent        RenderTargetExtent
	Context       VulkanContextDescriptor
	Image         NativePointer
	ImageView     NativePointer
	Format        uint32
	InitialLayout uint32
	FinalLayout   uint32
}

// WglContextDescriptor contains Windows OpenGL (WGL) context handles. The
// session creates a context in the same share group as ShareContext and uses
// DeviceContext for context creation. Handles are borrowed; the caller keeps
// them valid until detach or session close.
type WglContextDescriptor struct {
	DeviceContext  NativePointer // HDC
	ShareContext   NativePointer // HGLRC
	GetProcAddress NativePointer // optional wglGetProcAddress-compatible PFN
}

// EglContextDescriptor contains Linux OpenGL (EGL) context handles. The session
// creates a context in the same share group as ShareContext on Display using
// Config. Config must include EGL_PBUFFER_BIT in EGL_SURFACE_TYPE for OpenGL
// texture sessions. Handles are borrowed; the caller keeps them valid until
// detach or session close.
type EglContextDescriptor struct {
	Display        NativePointer // EGLDisplay
	Config         NativePointer // EGLConfig
	ShareContext   NativePointer // EGLContext
	GetProcAddress NativePointer // optional eglGetProcAddress-compatible PFN
}

// OpenGLContextDescriptor describes the OpenGL context provider data for an
// OpenGL render target. Construct with NewOpenGLContextWGL or
// NewOpenGLContextEGL — Platform must match the populated variant.
type OpenGLContextDescriptor struct {
	Platform OpenGLContextPlatform
	WGL      WglContextDescriptor
	EGL      EglContextDescriptor
}

// NewOpenGLContextWGL returns an OpenGL context descriptor backed by WGL.
func NewOpenGLContextWGL(descriptor WglContextDescriptor) OpenGLContextDescriptor {
	return OpenGLContextDescriptor{Platform: OpenGLContextPlatformWGL, WGL: descriptor}
}

// NewOpenGLContextEGL returns an OpenGL context descriptor backed by EGL.
func NewOpenGLContextEGL(descriptor EglContextDescriptor) OpenGLContextDescriptor {
	return OpenGLContextDescriptor{Platform: OpenGLContextPlatformEGL, EGL: descriptor}
}

// OpenGLOwnedTextureDescriptor describes a session-owned OpenGL texture render
// target. The session creates and owns a context that shares texture objects
// with the host context indicated by Context.
type OpenGLOwnedTextureDescriptor struct {
	Extent  RenderTargetExtent
	Context OpenGLContextDescriptor
}

// OpenGLBorrowedTextureDescriptor describes a caller-owned OpenGL texture
// render target. Texture must belong to Context (or a context in the same
// share group) and remain valid until detach or session close.
type OpenGLBorrowedTextureDescriptor struct {
	Extent  RenderTargetExtent
	Context OpenGLContextDescriptor
	Texture uint32 // OpenGL texture object name
	Target  uint32 // OpenGL texture target, typically GL_TEXTURE_2D
}

// OpenGLSurfaceDescriptor describes an OpenGL native surface render target.
// Surface is an HDC on WGL or an EGLSurface on EGL. The caller keeps Surface
// and Context handles valid until detach or session close.
type OpenGLSurfaceDescriptor struct {
	Extent  RenderTargetExtent
	Context OpenGLContextDescriptor
	Surface NativePointer
}

// RenderSessionHandle owns a map render session.
type RenderSessionHandle struct {
	state  *handle.State[nativeRenderSession]
	parent *MapHandle
	mu     sync.Mutex
	frame  bool
}

type metalOwnedTextureFrameState struct {
	session *RenderSessionHandle
	raw     C.mln_metal_owned_texture_frame
	mu      sync.Mutex
	closed  bool
}

type vulkanOwnedTextureFrameState struct {
	session *RenderSessionHandle
	raw     C.mln_vulkan_owned_texture_frame
	mu      sync.Mutex
	closed  bool
}

type openglOwnedTextureFrameState struct {
	session *RenderSessionHandle
	raw     C.mln_opengl_owned_texture_frame
	mu      sync.Mutex
	closed  bool
}

// MetalOwnedTextureFrame is an acquired session-owned Metal texture frame.
// Backend handles are borrowed and remain valid only until Close. Close the
// frame on the render session owner thread before resizing, rendering, reading
// back, detaching, closing the session, or acquiring another frame.
type MetalOwnedTextureFrame struct {
	Generation  uint64
	Width       uint32
	Height      uint32
	ScaleFactor float64
	Texture     NativePointer
	Device      NativePointer
	PixelFormat uint64

	state *metalOwnedTextureFrameState
}

// VulkanOwnedTextureFrame is an acquired session-owned Vulkan texture frame.
// Backend handles are borrowed and remain valid only until Close. Close the
// frame on the render session owner thread before resizing, rendering, reading
// back, detaching, closing the session, or acquiring another frame.
type VulkanOwnedTextureFrame struct {
	Generation  uint64
	Width       uint32
	Height      uint32
	ScaleFactor float64
	Image       NativePointer
	ImageView   NativePointer
	Device      NativePointer
	Format      uint32
	Layout      uint32

	state *vulkanOwnedTextureFrameState
}

// OpenGLOwnedTextureFrame is an acquired session-owned OpenGL texture frame.
// Texture is an OpenGL texture object name in the host context's share group
// and remains valid only until Close. Close the frame on the render session
// owner thread before resizing, rendering, reading back, detaching, closing
// the session, or acquiring another frame.
type OpenGLOwnedTextureFrame struct {
	Generation     uint64
	Width          uint32
	Height         uint32
	ScaleFactor    float64
	Texture        uint32 // OpenGL texture object name
	Target         uint32 // OpenGL texture target, typically GL_TEXTURE_2D
	InternalFormat uint32 // OpenGL internal format, such as GL_RGBA8
	Format         uint32 // OpenGL pixel format, such as GL_RGBA
	Type           uint32 // OpenGL pixel type, such as GL_UNSIGNED_BYTE

	state *openglOwnedTextureFrameState
}

func (extent RenderTargetExtent) toC() C.mln_render_target_extent {
	return C.mln_render_target_extent{
		size:         C.uint32_t(unsafe.Sizeof(C.mln_render_target_extent{})),
		width:        C.uint32_t(extent.Width),
		height:       C.uint32_t(extent.Height),
		scale_factor: C.double(extent.ScaleFactor),
	}
}

func textureImageInfoFromC(info C.mln_texture_image_info) TextureImageInfo {
	return TextureImageInfo{Width: uint32(info.width), Height: uint32(info.height), Stride: uint32(info.stride), ByteLength: uint64(info.byte_length)}
}

func (descriptor MetalSurfaceDescriptor) toC() C.mln_metal_surface_descriptor {
	raw := C.mln_metal_surface_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.context.device = unsafe.Pointer(uintptr(descriptor.Context.Device))
	raw.layer = unsafe.Pointer(uintptr(descriptor.Layer))
	return raw
}

func (descriptor VulkanSurfaceDescriptor) toC() C.mln_vulkan_surface_descriptor {
	raw := C.mln_vulkan_surface_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.context = descriptor.Context.toC()
	raw.surface = unsafe.Pointer(uintptr(descriptor.Surface))
	return raw
}

func (descriptor MetalOwnedTextureDescriptor) toC() C.mln_metal_owned_texture_descriptor {
	raw := C.mln_metal_owned_texture_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.context.device = unsafe.Pointer(uintptr(descriptor.Context.Device))
	return raw
}

func (descriptor MetalBorrowedTextureDescriptor) toC() C.mln_metal_borrowed_texture_descriptor {
	raw := C.mln_metal_borrowed_texture_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.texture = unsafe.Pointer(uintptr(descriptor.Texture))
	return raw
}

func (descriptor VulkanOwnedTextureDescriptor) toC() C.mln_vulkan_owned_texture_descriptor {
	raw := C.mln_vulkan_owned_texture_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.context = descriptor.Context.toC()
	return raw
}

func (descriptor VulkanBorrowedTextureDescriptor) toC() C.mln_vulkan_borrowed_texture_descriptor {
	raw := C.mln_vulkan_borrowed_texture_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.context = descriptor.Context.toC()
	raw.image = unsafe.Pointer(uintptr(descriptor.Image))
	raw.image_view = unsafe.Pointer(uintptr(descriptor.ImageView))
	raw.format = C.uint32_t(descriptor.Format)
	raw.initial_layout = C.uint32_t(descriptor.InitialLayout)
	raw.final_layout = C.uint32_t(descriptor.FinalLayout)
	return raw
}

func (context VulkanContextDescriptor) toC() C.mln_vulkan_context_descriptor {
	return C.mln_vulkan_context_descriptor{
		size:                        C.uint32_t(unsafe.Sizeof(C.mln_vulkan_context_descriptor{})),
		instance:                    unsafe.Pointer(uintptr(context.Instance)),
		physical_device:             unsafe.Pointer(uintptr(context.PhysicalDevice)),
		device:                      unsafe.Pointer(uintptr(context.Device)),
		graphics_queue:              unsafe.Pointer(uintptr(context.GraphicsQueue)),
		graphics_queue_family_index: C.uint32_t(context.GraphicsQueueFamilyIndex),
	}
}

func (context WglContextDescriptor) toC() C.mln_wgl_context_descriptor {
	return C.mln_wgl_context_descriptor{
		size:             C.uint32_t(unsafe.Sizeof(C.mln_wgl_context_descriptor{})),
		device_context:   unsafe.Pointer(uintptr(context.DeviceContext)),
		share_context:    unsafe.Pointer(uintptr(context.ShareContext)),
		get_proc_address: unsafe.Pointer(uintptr(context.GetProcAddress)),
	}
}

func (context EglContextDescriptor) toC() C.mln_egl_context_descriptor {
	return C.mln_egl_context_descriptor{
		size:             C.uint32_t(unsafe.Sizeof(C.mln_egl_context_descriptor{})),
		display:          unsafe.Pointer(uintptr(context.Display)),
		config:           unsafe.Pointer(uintptr(context.Config)),
		share_context:    unsafe.Pointer(uintptr(context.ShareContext)),
		get_proc_address: unsafe.Pointer(uintptr(context.GetProcAddress)),
	}
}

func (context OpenGLContextDescriptor) toC() C.mln_opengl_context_descriptor {
	switch context.Platform {
	case OpenGLContextPlatformWGL:
		return C.mln_go_opengl_context_descriptor_wgl(context.WGL.toC())
	case OpenGLContextPlatformEGL:
		return C.mln_go_opengl_context_descriptor_egl(context.EGL.toC())
	default:
		return C.mln_opengl_context_descriptor{
			size:     C.uint32_t(unsafe.Sizeof(C.mln_opengl_context_descriptor{})),
			platform: C.mln_opengl_context_platform(context.Platform),
		}
	}
}

func (descriptor OpenGLOwnedTextureDescriptor) toC() C.mln_opengl_owned_texture_descriptor {
	raw := C.mln_opengl_owned_texture_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.context = descriptor.Context.toC()
	return raw
}

func (descriptor OpenGLBorrowedTextureDescriptor) toC() C.mln_opengl_borrowed_texture_descriptor {
	raw := C.mln_opengl_borrowed_texture_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.context = descriptor.Context.toC()
	raw.texture = C.uint32_t(descriptor.Texture)
	raw.target = C.uint32_t(descriptor.Target)
	return raw
}

func (descriptor OpenGLSurfaceDescriptor) toC() C.mln_opengl_surface_descriptor {
	raw := C.mln_opengl_surface_descriptor_default()
	raw.extent = descriptor.Extent.toC()
	raw.context = descriptor.Context.toC()
	raw.surface = unsafe.Pointer(uintptr(descriptor.Surface))
	return raw
}

func newRenderSessionHandle(parent *MapHandle, session *nativeRenderSession) (*RenderSessionHandle, error) {
	state, err := handle.New(session, "RenderSessionHandle", parent)
	if err != nil {
		return nil, newBindingError(ErrInvalidArgument, err.Error())
	}
	return &RenderSessionHandle{state: state, parent: parent}, nil
}

// AttachMetalSurface attaches a Metal native surface render target to this map.
// The returned session is owner-thread affine to the map owner thread.
func (m *MapHandle) AttachMetalSurface(descriptor MetalSurfaceDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_metal_surface_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

// AttachVulkanSurface attaches a Vulkan native surface render target to this
// map. The returned session is owner-thread affine to the map owner thread, and
// borrowed Vulkan handles must outlive detach or session close.
func (m *MapHandle) AttachVulkanSurface(descriptor VulkanSurfaceDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_vulkan_surface_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

// AttachMetalOwnedTexture attaches a Metal session-owned texture render target.
func (m *MapHandle) AttachMetalOwnedTexture(descriptor MetalOwnedTextureDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_metal_owned_texture_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

// AttachMetalBorrowedTexture attaches a Metal caller-owned texture render
// target. The caller keeps the texture valid until detach or session close and
// synchronizes external use.
func (m *MapHandle) AttachMetalBorrowedTexture(descriptor MetalBorrowedTextureDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_metal_borrowed_texture_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

// AttachVulkanOwnedTexture attaches a Vulkan session-owned texture render target.
func (m *MapHandle) AttachVulkanOwnedTexture(descriptor VulkanOwnedTextureDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_vulkan_owned_texture_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

// AttachVulkanBorrowedTexture attaches a Vulkan caller-owned texture render
// target. The caller owns image lifetime, queue-family ownership, image layout
// transitions, and external synchronization around each RenderUpdate.
func (m *MapHandle) AttachVulkanBorrowedTexture(descriptor VulkanBorrowedTextureDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_vulkan_borrowed_texture_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

// AttachOpenGLSurface attaches an OpenGL native surface render target to this
// map. Surface is an HDC on WGL or an EGLSurface on EGL. The returned session
// is owner-thread affine to the map owner thread.
func (m *MapHandle) AttachOpenGLSurface(descriptor OpenGLSurfaceDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_opengl_surface_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

// AttachOpenGLOwnedTexture attaches an OpenGL session-owned texture render
// target. The session creates its own OpenGL context in the same share group
// as Context.WGL.ShareContext or Context.EGL.ShareContext.
func (m *MapHandle) AttachOpenGLOwnedTexture(descriptor OpenGLOwnedTextureDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_opengl_owned_texture_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

// AttachOpenGLBorrowedTexture attaches an OpenGL caller-owned texture render
// target. Texture must belong to Context (or a context in the same share
// group) and remain valid until detach or session close.
func (m *MapHandle) AttachOpenGLBorrowedTexture(descriptor OpenGLBorrowedTextureDescriptor) (*RenderSessionHandle, error) {
	ptr, err := m.ptr()
	if err != nil {
		return nil, err
	}
	defer m.state.KeepAlive()

	var session *C.mln_render_session
	rawDescriptor := descriptor.toC()
	if err := checkNative(func() int32 {
		return int32(C.mln_opengl_borrowed_texture_attach((*C.mln_map)(unsafe.Pointer(ptr)), &rawDescriptor, &session))
	}); err != nil {
		return nil, err
	}
	return newRenderSessionHandle(m, (*nativeRenderSession)(unsafe.Pointer(session)))
}

func (session *RenderSessionHandle) ptr() (*nativeRenderSession, error) {
	if session == nil || session.state == nil {
		return nil, newBindingError(ErrInvalidArgument, "RenderSessionHandle is nil")
	}
	ptr, live := session.state.Ptr()
	if !live {
		return nil, newBindingError(ErrInvalidArgument, "RenderSessionHandle is closed")
	}
	return ptr, nil
}

func (session *RenderSessionHandle) withNoAcquiredFrame(call func() error) error {
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.frame {
		return newBindingError(ErrInvalidState, "texture frame is still acquired")
	}
	return call()
}

func (session *RenderSessionHandle) markFrameReleased() {
	session.mu.Lock()
	session.frame = false
	session.mu.Unlock()
}

// Resize changes the render session target extent.
func (session *RenderSessionHandle) Resize(extent RenderTargetExtent) error {
	ptr, err := session.ptr()
	if err != nil {
		return err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	return session.withNoAcquiredFrame(func() error {
		return checkNative(func() int32 {
			return int32(C.mln_render_session_resize((*C.mln_render_session)(unsafe.Pointer(ptr)), C.uint32_t(extent.Width), C.uint32_t(extent.Height), C.double(extent.ScaleFactor)))
		})
	})
}

// RenderUpdate renders one frame/update into the attached render target.
func (session *RenderSessionHandle) RenderUpdate() error {
	ptr, err := session.ptr()
	if err != nil {
		return err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	return session.withNoAcquiredFrame(func() error {
		return checkNative(func() int32 {
			return int32(C.mln_render_session_render_update((*C.mln_render_session)(unsafe.Pointer(ptr))))
		})
	})
}

// Detach detaches the render target from the session.
func (session *RenderSessionHandle) Detach() error {
	ptr, err := session.ptr()
	if err != nil {
		return err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	return session.withNoAcquiredFrame(func() error {
		return checkNative(func() int32 { return int32(C.mln_render_session_detach((*C.mln_render_session)(unsafe.Pointer(ptr)))) })
	})
}

// ReduceMemoryUse asks the render session to release cached render resources.
func (session *RenderSessionHandle) ReduceMemoryUse() error {
	ptr, err := session.ptr()
	if err != nil {
		return err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	return checkNative(func() int32 {
		return int32(C.mln_render_session_reduce_memory_use((*C.mln_render_session)(unsafe.Pointer(ptr))))
	})
}

// ClearData clears render-session data.
func (session *RenderSessionHandle) ClearData() error {
	ptr, err := session.ptr()
	if err != nil {
		return err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	return checkNative(func() int32 {
		return int32(C.mln_render_session_clear_data((*C.mln_render_session)(unsafe.Pointer(ptr))))
	})
}

// DumpDebugLogs dumps render-session debug logs.
func (session *RenderSessionHandle) DumpDebugLogs() error {
	ptr, err := session.ptr()
	if err != nil {
		return err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	return checkNative(func() int32 {
		return int32(C.mln_render_session_dump_debug_logs((*C.mln_render_session)(unsafe.Pointer(ptr))))
	})
}

// ReadPremultipliedRGBA8 reads the latest session-owned texture frame into a
// new byte slice.
func (session *RenderSessionHandle) ReadPremultipliedRGBA8() ([]byte, TextureImageInfo, error) {
	ptr, err := session.ptr()
	if err != nil {
		return nil, TextureImageInfo{}, err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()

	var buffer []byte
	var info TextureImageInfo
	err = session.withNoAcquiredFrame(func() error {
		rawInfo := C.mln_texture_image_info_default()
		failure := internalstatus.CheckCall(func() int32 {
			return int32(C.mln_texture_read_premultiplied_rgba8((*C.mln_render_session)(unsafe.Pointer(ptr)), nil, 0, &rawInfo))
		}, threadLastErrorMessage)
		info = textureImageInfoFromC(rawInfo)
		if failure == nil {
			return nil
		}
		if failure.Status != int32(C.MLN_STATUS_INVALID_ARGUMENT) || info.ByteLength == 0 {
			return newStatusError(failure)
		}
		buffer = make([]byte, info.ByteLength)
		info, err = session.readPremultipliedRGBA8IntoLocked(ptr, buffer)
		return err
	})
	if err != nil {
		return nil, info, err
	}
	return buffer, info, nil
}

// ReadPremultipliedRGBA8Into reads the latest session-owned texture frame into
// caller-owned storage.
func (session *RenderSessionHandle) ReadPremultipliedRGBA8Into(buffer []byte) (TextureImageInfo, error) {
	ptr, err := session.ptr()
	if err != nil {
		return TextureImageInfo{}, err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	var info TextureImageInfo
	err = session.withNoAcquiredFrame(func() error {
		var readErr error
		info, readErr = session.readPremultipliedRGBA8IntoLocked(ptr, buffer)
		return readErr
	})
	return info, err
}

func (session *RenderSessionHandle) readPremultipliedRGBA8IntoLocked(ptr *nativeRenderSession, buffer []byte) (TextureImageInfo, error) {
	rawInfo := C.mln_texture_image_info_default()
	var rawBuffer *C.uint8_t
	if len(buffer) > 0 {
		rawBuffer = (*C.uint8_t)(unsafe.Pointer(&buffer[0]))
	}
	if err := checkNative(func() int32 {
		return int32(C.mln_texture_read_premultiplied_rgba8((*C.mln_render_session)(unsafe.Pointer(ptr)), rawBuffer, C.size_t(len(buffer)), &rawInfo))
	}); err != nil {
		runtime.KeepAlive(buffer)
		return textureImageInfoFromC(rawInfo), err
	}
	runtime.KeepAlive(buffer)
	return textureImageInfoFromC(rawInfo), nil
}

// AcquireMetalTextureFrame acquires the latest Metal session-owned texture
// frame. While the frame is live, resize, render update, detach, readback,
// session close, and another frame acquire are invalid.
func (session *RenderSessionHandle) AcquireMetalTextureFrame() (*MetalOwnedTextureFrame, error) {
	ptr, err := session.ptr()
	if err != nil {
		return nil, err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.frame {
		return nil, newBindingError(ErrInvalidState, "texture frame is still acquired")
	}
	rawFrame := C.mln_metal_owned_texture_frame{size: C.uint32_t(unsafe.Sizeof(C.mln_metal_owned_texture_frame{}))}
	if err := checkNative(func() int32 {
		return int32(C.mln_metal_owned_texture_acquire_frame((*C.mln_render_session)(unsafe.Pointer(ptr)), &rawFrame))
	}); err != nil {
		return nil, err
	}
	session.frame = true
	return &MetalOwnedTextureFrame{
		Generation:  uint64(rawFrame.generation),
		Width:       uint32(rawFrame.width),
		Height:      uint32(rawFrame.height),
		ScaleFactor: float64(rawFrame.scale_factor),
		Texture:     NativePointer(uintptr(rawFrame.texture)),
		Device:      NativePointer(uintptr(rawFrame.device)),
		PixelFormat: uint64(rawFrame.pixel_format),
		state:       &metalOwnedTextureFrameState{session: session, raw: rawFrame},
	}, nil
}

// AcquireOpenGLTextureFrame acquires the latest OpenGL session-owned texture
// frame. While the frame is live, resize, render update, detach, readback,
// session close, and another frame acquire are invalid.
func (session *RenderSessionHandle) AcquireOpenGLTextureFrame() (*OpenGLOwnedTextureFrame, error) {
	ptr, err := session.ptr()
	if err != nil {
		return nil, err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.frame {
		return nil, newBindingError(ErrInvalidState, "texture frame is still acquired")
	}
	rawFrame := C.mln_opengl_owned_texture_frame{size: C.uint32_t(unsafe.Sizeof(C.mln_opengl_owned_texture_frame{}))}
	if err := checkNative(func() int32 {
		return int32(C.mln_opengl_owned_texture_acquire_frame((*C.mln_render_session)(unsafe.Pointer(ptr)), &rawFrame))
	}); err != nil {
		return nil, err
	}
	session.frame = true
	return &OpenGLOwnedTextureFrame{
		Generation:     uint64(rawFrame.generation),
		Width:          uint32(rawFrame.width),
		Height:         uint32(rawFrame.height),
		ScaleFactor:    float64(rawFrame.scale_factor),
		Texture:        uint32(rawFrame.texture),
		Target:         uint32(rawFrame.target),
		InternalFormat: uint32(rawFrame.internal_format),
		Format:         uint32(rawFrame.format),
		Type:           uint32(rawFrame._type),
		state:          &openglOwnedTextureFrameState{session: session, raw: rawFrame},
	}, nil
}

// AcquireVulkanTextureFrame acquires the latest Vulkan session-owned texture
// frame. While the frame is live, resize, render update, detach, readback,
// session close, and another frame acquire are invalid.
func (session *RenderSessionHandle) AcquireVulkanTextureFrame() (*VulkanOwnedTextureFrame, error) {
	ptr, err := session.ptr()
	if err != nil {
		return nil, err
	}
	defer session.state.KeepAlive()
	defer session.parent.state.KeepAlive()
	session.mu.Lock()
	defer session.mu.Unlock()
	if session.frame {
		return nil, newBindingError(ErrInvalidState, "texture frame is still acquired")
	}
	rawFrame := C.mln_vulkan_owned_texture_frame{size: C.uint32_t(unsafe.Sizeof(C.mln_vulkan_owned_texture_frame{}))}
	if err := checkNative(func() int32 {
		return int32(C.mln_vulkan_owned_texture_acquire_frame((*C.mln_render_session)(unsafe.Pointer(ptr)), &rawFrame))
	}); err != nil {
		return nil, err
	}
	session.frame = true
	return &VulkanOwnedTextureFrame{
		Generation:  uint64(rawFrame.generation),
		Width:       uint32(rawFrame.width),
		Height:      uint32(rawFrame.height),
		ScaleFactor: float64(rawFrame.scale_factor),
		Image:       NativePointer(uintptr(rawFrame.image)),
		ImageView:   NativePointer(uintptr(rawFrame.image_view)),
		Device:      NativePointer(uintptr(rawFrame.device)),
		Format:      uint32(rawFrame.format),
		Layout:      uint32(rawFrame.layout),
		state:       &vulkanOwnedTextureFrameState{session: session, raw: rawFrame},
	}, nil
}

// Close releases this acquired Metal texture frame on the session owner thread.
// A second Close is a no-op after a successful release; failed releases remain
// retryable.
func (frame *MetalOwnedTextureFrame) Close() error {
	if frame == nil || frame.state == nil || frame.state.session == nil {
		return newBindingError(ErrInvalidArgument, "MetalOwnedTextureFrame is nil")
	}
	state := frame.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil
	}
	ptr, err := state.session.ptr()
	if err != nil {
		return err
	}
	defer state.session.state.KeepAlive()
	defer state.session.parent.state.KeepAlive()
	if err := checkNative(func() int32 {
		return int32(C.mln_metal_owned_texture_release_frame((*C.mln_render_session)(unsafe.Pointer(ptr)), &state.raw))
	}); err != nil {
		return err
	}
	state.closed = true
	state.session.markFrameReleased()
	return nil
}

// Close releases this acquired OpenGL texture frame on the session owner
// thread. A second Close is a no-op after a successful release; failed releases
// remain retryable.
func (frame *OpenGLOwnedTextureFrame) Close() error {
	if frame == nil || frame.state == nil || frame.state.session == nil {
		return newBindingError(ErrInvalidArgument, "OpenGLOwnedTextureFrame is nil")
	}
	state := frame.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil
	}
	ptr, err := state.session.ptr()
	if err != nil {
		return err
	}
	defer state.session.state.KeepAlive()
	defer state.session.parent.state.KeepAlive()
	if err := checkNative(func() int32 {
		return int32(C.mln_opengl_owned_texture_release_frame((*C.mln_render_session)(unsafe.Pointer(ptr)), &state.raw))
	}); err != nil {
		return err
	}
	state.closed = true
	state.session.markFrameReleased()
	return nil
}

// Close releases this acquired Vulkan texture frame on the session owner
// thread. A second Close is a no-op after a successful release; failed releases
// remain retryable.
func (frame *VulkanOwnedTextureFrame) Close() error {
	if frame == nil || frame.state == nil || frame.state.session == nil {
		return newBindingError(ErrInvalidArgument, "VulkanOwnedTextureFrame is nil")
	}
	state := frame.state
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.closed {
		return nil
	}
	ptr, err := state.session.ptr()
	if err != nil {
		return err
	}
	defer state.session.state.KeepAlive()
	defer state.session.parent.state.KeepAlive()
	if err := checkNative(func() int32 {
		return int32(C.mln_vulkan_owned_texture_release_frame((*C.mln_render_session)(unsafe.Pointer(ptr)), &state.raw))
	}); err != nil {
		return err
	}
	state.closed = true
	state.session.markFrameReleased()
	return nil
}

// Close destroys this render session. A successful close makes later calls
// no-ops. A failed close leaves the native handle live so callers can retry on
// the owner thread.
func (session *RenderSessionHandle) Close() error {
	if session == nil || session.state == nil {
		return newBindingError(ErrInvalidArgument, "RenderSessionHandle is nil")
	}
	defer session.parent.state.KeepAlive()
	return session.withNoAcquiredFrame(func() error {
		return checkNative(func() int32 {
			return session.state.Close(func(ptr *nativeRenderSession) int32 {
				return int32(C.mln_render_session_destroy((*C.mln_render_session)(unsafe.Pointer(ptr))))
			})
		})
	})
}
