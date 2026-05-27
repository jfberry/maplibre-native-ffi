package maplibre

import "testing"

func TestCVersionUsesNativeABI(t *testing.T) {
	if got := CVersion(); got != 0 {
		t.Fatalf("CVersion() = %d, want 0 while ABI is unstable", got)
	}
}
func TestSupportedRenderBackendsUsesNativeABIConstants(t *testing.T) {
	mask := SupportedRenderBackends()
	if mask == 0 {
		t.Fatal("SupportedRenderBackends() returned empty mask")
	}
	if mask.Has(RenderBackendMetal) && uint32(RenderBackendMetal) == 0 {
		t.Fatal("RenderBackendMetal has zero ABI value")
	}
	if mask.Has(RenderBackendOpenGL) && uint32(RenderBackendOpenGL) == 0 {
		t.Fatal("RenderBackendOpenGL has zero ABI value")
	}
	if mask.Has(RenderBackendVulkan) && uint32(RenderBackendVulkan) == 0 {
		t.Fatal("RenderBackendVulkan has zero ABI value")
	}
}
func TestSupportedOpenGLContextProvidersUsesNativeABIConstants(t *testing.T) {
	mask := SupportedOpenGLContextProviders()
	if mask.Has(OpenGLContextProviderWGL) && uint32(OpenGLContextProviderWGL) == 0 {
		t.Fatal("OpenGLContextProviderWGL has zero ABI value")
	}
	if mask.Has(OpenGLContextProviderEGL) && uint32(OpenGLContextProviderEGL) == 0 {
		t.Fatal("OpenGLContextProviderEGL has zero ABI value")
	}
	if SupportedRenderBackends().Has(RenderBackendOpenGL) && mask == 0 {
		t.Fatal("OpenGL backend present but no context providers reported")
	}
}
func TestNativePointerIsOpaqueValue(t *testing.T) {
	var pointer NativePointer = 0x1234
	if uintptr(pointer) != 0x1234 {
		t.Fatalf("NativePointer preserved address value %x", uintptr(pointer))
	}
}
