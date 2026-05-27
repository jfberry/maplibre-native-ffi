package maplibre

import (
	"errors"
	"testing"
)

func TestRenderSessionNilHandleAndInvalidSurfaceDescriptor(t *testing.T) {
	var nilSession *RenderSessionHandle
	if err := nilSession.Close(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil RenderSessionHandle Close() error = %v, want ErrInvalidArgument", err)
	}
	if err := nilSession.RenderUpdate(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil RenderSessionHandle RenderUpdate() error = %v, want ErrInvalidArgument", err)
	}
	if _, err := nilSession.ReadPremultipliedRGBA8Into(nil); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil RenderSessionHandle ReadPremultipliedRGBA8Into() error = %v, want ErrInvalidArgument", err)
	}
	var nilMetalFrame *MetalOwnedTextureFrame
	if err := nilMetalFrame.Close(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil MetalOwnedTextureFrame Close() error = %v, want ErrInvalidArgument", err)
	}
	var nilVulkanFrame *VulkanOwnedTextureFrame
	if err := nilVulkanFrame.Close(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil VulkanOwnedTextureFrame Close() error = %v, want ErrInvalidArgument", err)
	}
	var nilOpenGLFrame *OpenGLOwnedTextureFrame
	if err := nilOpenGLFrame.Close(); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("nil OpenGLOwnedTextureFrame Close() error = %v, want ErrInvalidArgument", err)
	}

	runtime, err := NewRuntime()
	if err != nil {
		t.Fatalf("NewRuntime(): %v", err)
	}
	m, err := runtime.NewMapWithOptions(NewMapOptions(64, 64, 1))
	if err != nil {
		_ = runtime.Close()
		t.Fatalf("NewMapWithOptions(): %v", err)
	}
	defer func() {
		if err := m.Close(); err != nil {
			t.Errorf("Map Close(): %v", err)
		}
		if err := runtime.Close(); err != nil {
			t.Errorf("Runtime Close(): %v", err)
		}
	}()

	_, err = m.AttachMetalSurface(MetalSurfaceDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachMetalSurface(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
	_, err = m.AttachMetalOwnedTexture(MetalOwnedTextureDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachMetalOwnedTexture(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
	_, err = m.AttachMetalBorrowedTexture(MetalBorrowedTextureDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachMetalBorrowedTexture(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
	_, err = m.AttachVulkanOwnedTexture(VulkanOwnedTextureDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachVulkanOwnedTexture(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
	_, err = m.AttachVulkanBorrowedTexture(VulkanBorrowedTextureDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachVulkanBorrowedTexture(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
	_, err = m.AttachOpenGLSurface(OpenGLSurfaceDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachOpenGLSurface(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
	_, err = m.AttachOpenGLOwnedTexture(OpenGLOwnedTextureDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachOpenGLOwnedTexture(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
	_, err = m.AttachOpenGLBorrowedTexture(OpenGLBorrowedTextureDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachOpenGLBorrowedTexture(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
	_, err = m.AttachOpenGLOffscreen(OpenGLOffscreenDescriptor{
		Extent: RenderTargetExtent{Width: 64, Height: 64, ScaleFactor: 1},
	})
	if !errors.Is(err, ErrInvalidArgument) && !errors.Is(err, ErrUnsupported) {
		t.Fatalf("AttachOpenGLOffscreen(invalid descriptor) error = %v, want ErrInvalidArgument or ErrUnsupported", err)
	}
}

func TestNewOpenGLContextConstructorsSetPlatform(t *testing.T) {
	wgl := NewOpenGLContextWGL(WglContextDescriptor{
		DeviceContext: NativePointer(0xCAFE),
		ShareContext:  NativePointer(0xBEEF),
	})
	if wgl.Platform != OpenGLContextPlatformWGL {
		t.Fatalf("NewOpenGLContextWGL platform = %v, want WGL", wgl.Platform)
	}
	if wgl.WGL.DeviceContext != NativePointer(0xCAFE) || wgl.WGL.ShareContext != NativePointer(0xBEEF) {
		t.Fatalf("NewOpenGLContextWGL did not preserve descriptor fields")
	}
	if wgl.EGL != (EglContextDescriptor{}) {
		t.Fatalf("NewOpenGLContextWGL populated EGL variant")
	}
	egl := NewOpenGLContextEGL(EglContextDescriptor{
		Display:      NativePointer(0x1111),
		Config:       NativePointer(0x2222),
		ShareContext: NativePointer(0x3333),
	})
	if egl.Platform != OpenGLContextPlatformEGL {
		t.Fatalf("NewOpenGLContextEGL platform = %v, want EGL", egl.Platform)
	}
	if egl.EGL.Display != NativePointer(0x1111) || egl.EGL.Config != NativePointer(0x2222) ||
		egl.EGL.ShareContext != NativePointer(0x3333) {
		t.Fatalf("NewOpenGLContextEGL did not preserve descriptor fields")
	}
	if egl.WGL != (WglContextDescriptor{}) {
		t.Fatalf("NewOpenGLContextEGL populated WGL variant")
	}
}
