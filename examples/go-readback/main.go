//go:build linux

// Command go-readback drives the Go binding's OpenGL EGL render path end to
// end: it sets up a surfaceless EGL context with a pbuffer surface, attaches a
// session-owned OpenGL texture render target to a static map, renders one
// frame from a background-only style, reads the premultiplied RGBA8 pixels
// back, and writes a PPM image. Run via mise with MISE_ENV=linux-*-egl.
package main

/*
#cgo linux pkg-config: egl

#include <EGL/egl.h>
#include <stddef.h>
#include <stdio.h>
#include <string.h>

typedef struct mln_go_egl_context {
    EGLDisplay display;
    EGLConfig  config;
    EGLContext share_context;
    EGLSurface surface;
} mln_go_egl_context;

static int mln_go_egl_init(mln_go_egl_context *out, char *err, size_t err_len) {
    memset(out, 0, sizeof(*out));

    out->display = eglGetDisplay(EGL_DEFAULT_DISPLAY);
    if (out->display == EGL_NO_DISPLAY) {
        snprintf(err, err_len, "eglGetDisplay returned EGL_NO_DISPLAY (0x%x)", eglGetError());
        return -1;
    }
    EGLint major = 0, minor = 0;
    if (eglInitialize(out->display, &major, &minor) == EGL_FALSE) {
        snprintf(err, err_len, "eglInitialize failed (0x%x)", eglGetError());
        return -2;
    }
    if (eglBindAPI(EGL_OPENGL_ES_API) == EGL_FALSE) {
        snprintf(err, err_len, "eglBindAPI(EGL_OPENGL_ES_API) failed (0x%x)", eglGetError());
        return -3;
    }

    EGLint config_attribs[] = {
        EGL_SURFACE_TYPE,    EGL_PBUFFER_BIT,
        EGL_RENDERABLE_TYPE, EGL_OPENGL_ES3_BIT,
        EGL_RED_SIZE,        8,
        EGL_GREEN_SIZE,      8,
        EGL_BLUE_SIZE,       8,
        EGL_ALPHA_SIZE,      8,
        EGL_DEPTH_SIZE,      24,
        EGL_STENCIL_SIZE,    8,
        EGL_NONE
    };
    EGLint config_count = 0;
    if (eglChooseConfig(out->display, config_attribs, &out->config, 1, &config_count) == EGL_FALSE ||
        config_count == 0 || out->config == NULL) {
        snprintf(err, err_len, "eglChooseConfig found no config (0x%x)", eglGetError());
        return -4;
    }

    EGLint context_attribs[] = {
        EGL_CONTEXT_CLIENT_VERSION, 3,
        EGL_NONE
    };
    out->share_context = eglCreateContext(out->display, out->config, EGL_NO_CONTEXT, context_attribs);
    if (out->share_context == EGL_NO_CONTEXT) {
        snprintf(err, err_len, "eglCreateContext failed (0x%x)", eglGetError());
        return -5;
    }

    EGLint surface_attribs[] = {
        EGL_WIDTH,  8,
        EGL_HEIGHT, 8,
        EGL_NONE
    };
    out->surface = eglCreatePbufferSurface(out->display, out->config, surface_attribs);
    if (out->surface == EGL_NO_SURFACE) {
        snprintf(err, err_len, "eglCreatePbufferSurface failed (0x%x)", eglGetError());
        return -6;
    }

    if (eglMakeCurrent(out->display, out->surface, out->surface, out->share_context) == EGL_FALSE) {
        snprintf(err, err_len, "eglMakeCurrent failed (0x%x)", eglGetError());
        return -7;
    }
    return 0;
}

static void mln_go_egl_destroy(mln_go_egl_context *ctx) {
    if (ctx == NULL || ctx->display == EGL_NO_DISPLAY) return;
    eglMakeCurrent(ctx->display, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT);
    if (ctx->surface != EGL_NO_SURFACE) eglDestroySurface(ctx->display, ctx->surface);
    if (ctx->share_context != EGL_NO_CONTEXT) eglDestroyContext(ctx->display, ctx->share_context);
    eglTerminate(ctx->display);
    memset(ctx, 0, sizeof(*ctx));
}

static void *mln_go_egl_get_proc_address(void) {
    return (void *)eglGetProcAddress;
}
*/
import "C"

import (
	"errors"
	"fmt"
	"log"
	"os"
	"time"
	"unsafe"

	maplibre "github.com/maplibre/maplibre-native-ffi/bindings/go"
)

const (
	width        = 256
	height       = 256
	scaleFactor  = 1.0
	renderBudget = 10 * time.Second
)

func main() {
	output := "go-readback.ppm"
	target := "owned-texture" // owned-texture | offscreen
	for _, arg := range os.Args[1:] {
		switch arg {
		case "owned-texture", "offscreen":
			target = arg
		default:
			output = arg
		}
	}

	if err := run(output, target); err != nil {
		log.Fatalf("go-readback: %v", err)
	}
}

func run(outputPath, target string) error {
	backends := maplibre.SupportedRenderBackends()
	log.Printf("native render backends mask: 0x%x (metal=%t opengl=%t vulkan=%t)",
		uint32(backends),
		backends.Has(maplibre.RenderBackendMetal),
		backends.Has(maplibre.RenderBackendOpenGL),
		backends.Has(maplibre.RenderBackendVulkan))
	if !backends.Has(maplibre.RenderBackendOpenGL) {
		return errors.New("OpenGL backend not present in linked native library")
	}
	providers := maplibre.SupportedOpenGLContextProviders()
	log.Printf("opengl context providers mask: 0x%x (wgl=%t egl=%t)",
		uint32(providers),
		providers.Has(maplibre.OpenGLContextProviderWGL),
		providers.Has(maplibre.OpenGLContextProviderEGL))
	if !providers.Has(maplibre.OpenGLContextProviderEGL) {
		return errors.New("EGL context provider not present in linked native library")
	}

	var egl C.mln_go_egl_context
	var errBuf [256]C.char
	if rc := C.mln_go_egl_init(&egl, &errBuf[0], C.size_t(len(errBuf))); rc != 0 {
		return fmt.Errorf("EGL init: %s", C.GoString(&errBuf[0]))
	}
	defer C.mln_go_egl_destroy(&egl)

	runtime, err := maplibre.NewRuntime()
	if err != nil {
		return fmt.Errorf("NewRuntime: %w", err)
	}
	defer runtime.Close()

	m, err := runtime.NewMapWithOptions(maplibre.MapOptions{
		Width:       width,
		Height:      height,
		ScaleFactor: scaleFactor,
		Mode:        maplibre.MapModeStatic,
	})
	if err != nil {
		return fmt.Errorf("NewMapWithOptions: %w", err)
	}
	defer m.Close()

	context := maplibre.NewOpenGLContextEGL(maplibre.EglContextDescriptor{
		Display:        maplibre.NativePointer(uintptr(unsafe.Pointer(egl.display))),
		Config:         maplibre.NativePointer(uintptr(unsafe.Pointer(egl.config))),
		ShareContext:   maplibre.NativePointer(uintptr(unsafe.Pointer(egl.share_context))),
		GetProcAddress: maplibre.NativePointer(uintptr(C.mln_go_egl_get_proc_address())),
	})
	extent := maplibre.RenderTargetExtent{Width: width, Height: height, ScaleFactor: scaleFactor}
	var session *maplibre.RenderSessionHandle
	switch target {
	case "offscreen":
		log.Printf("attach: OpenGL offscreen FBO (renderbuffer color, HeadlessBackend layout)")
		session, err = m.AttachOpenGLOffscreen(maplibre.OpenGLOffscreenDescriptor{
			Extent:  extent,
			Context: context,
		})
		if err != nil {
			return fmt.Errorf("AttachOpenGLOffscreen: %w", err)
		}
	case "owned-texture":
		log.Printf("attach: OpenGL owned texture")
		session, err = m.AttachOpenGLOwnedTexture(maplibre.OpenGLOwnedTextureDescriptor{
			Extent:  extent,
			Context: context,
		})
		if err != nil {
			return fmt.Errorf("AttachOpenGLOwnedTexture: %w", err)
		}
	default:
		return fmt.Errorf("unknown target %q (want owned-texture | offscreen)", target)
	}
	defer session.Close()

	if err := m.SetStyleJSON(`{
		"version": 8,
		"name": "go-readback",
		"sources": {},
		"layers": [{
			"id": "bg",
			"type": "background",
			"paint": {"background-color": "#FF8800"}
		}]
	}`); err != nil {
		return fmt.Errorf("SetStyleJSON: %w", err)
	}

	camera := maplibre.CameraOptions{}.
		WithCenter(maplibre.LatLng{Latitude: 0, Longitude: 0}).
		WithZoom(0)
	if err := m.JumpTo(camera); err != nil {
		return fmt.Errorf("JumpTo: %w", err)
	}

	if err := m.RequestStillImage(); err != nil {
		return fmt.Errorf("RequestStillImage: %w", err)
	}

	if err := pumpUntilStillImageFinished(runtime, session, renderBudget); err != nil {
		return err
	}

	pixels, info, err := session.ReadPremultipliedRGBA8()
	if err != nil {
		return fmt.Errorf("ReadPremultipliedRGBA8: %w", err)
	}
	log.Printf("readback: %dx%d stride=%d byteLength=%d", info.Width, info.Height, info.Stride, info.ByteLength)

	if err := writePPM(outputPath, pixels, info); err != nil {
		return fmt.Errorf("writePPM: %w", err)
	}
	log.Printf("wrote %s (%dx%d)", outputPath, info.Width, info.Height)

	if err := verifyNonDegenerate(pixels, info); err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	log.Printf("verified: pixels are non-zero and dominantly orange-ish (matches background-color #FF8800)")
	return nil
}

func pumpUntilStillImageFinished(runtime *maplibre.RuntimeHandle, session *maplibre.RenderSessionHandle, budget time.Duration) error {
	rendered := false
	deadline := time.Now().Add(budget)
	for time.Now().Before(deadline) {
		if err := runtime.RunOnce(); err != nil {
			return fmt.Errorf("RunOnce: %w", err)
		}
		for {
			event, err := runtime.PollEvent()
			if err != nil {
				return fmt.Errorf("PollEvent: %w", err)
			}
			if event == nil {
				break
			}
			switch event.Type {
			case maplibre.RuntimeEventMapRenderUpdateAvailable:
				if err := session.RenderUpdate(); err != nil {
					return fmt.Errorf("RenderUpdate: %w", err)
				}
				rendered = true
			case maplibre.RuntimeEventMapStillImageFinished:
				if !rendered {
					return errors.New("still image finished without a render frame")
				}
				return nil
			case maplibre.RuntimeEventMapLoadingFailed:
				return fmt.Errorf("map loading failed: %s", event.Message)
			case maplibre.RuntimeEventMapRenderError:
				return fmt.Errorf("map render error: %s", event.Message)
			case maplibre.RuntimeEventMapStillImageFailed:
				return fmt.Errorf("still image failed: %s", event.Message)
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %s waiting for still image", budget)
}

func writePPM(path string, pixels []byte, info maplibre.TextureImageInfo) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	header := fmt.Sprintf("P6\n%d %d\n255\n", info.Width, info.Height)
	if _, err := f.WriteString(header); err != nil {
		return err
	}
	row := make([]byte, info.Width*3)
	stride := int(info.Stride)
	if stride == 0 {
		stride = int(info.Width) * 4
	}
	for y := uint32(0); y < info.Height; y++ {
		base := int(y) * stride
		for x := uint32(0); x < info.Width; x++ {
			row[x*3+0] = pixels[base+int(x)*4+0]
			row[x*3+1] = pixels[base+int(x)*4+1]
			row[x*3+2] = pixels[base+int(x)*4+2]
		}
		if _, err := f.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func verifyNonDegenerate(pixels []byte, info maplibre.TextureImageInfo) error {
	if uint64(len(pixels)) != info.ByteLength {
		return fmt.Errorf("expected %d bytes, got %d", info.ByteLength, len(pixels))
	}
	var nonZero int
	var rSum, gSum, bSum uint64
	stride := int(info.Stride)
	if stride == 0 {
		stride = int(info.Width) * 4
	}
	for y := uint32(0); y < info.Height; y++ {
		base := int(y) * stride
		for x := uint32(0); x < info.Width; x++ {
			r := pixels[base+int(x)*4+0]
			g := pixels[base+int(x)*4+1]
			b := pixels[base+int(x)*4+2]
			if r != 0 || g != 0 || b != 0 {
				nonZero++
			}
			rSum += uint64(r)
			gSum += uint64(g)
			bSum += uint64(b)
		}
	}
	total := int(info.Width) * int(info.Height)
	if nonZero*4 < total*3 {
		return fmt.Errorf("only %d/%d pixels are non-zero — render did not paint the background", nonZero, total)
	}
	rAvg := float64(rSum) / float64(total)
	gAvg := float64(gSum) / float64(total)
	bAvg := float64(bSum) / float64(total)
	log.Printf("channel averages: R=%.1f G=%.1f B=%.1f (expecting roughly R≈255 G≈136 B≈0 for #FF8800)", rAvg, gAvg, bAvg)
	if !(rAvg > gAvg && gAvg > bAvg) {
		return fmt.Errorf("channel ordering R>G>B not observed (R=%.1f G=%.1f B=%.1f) — background style did not paint", rAvg, gAvg, bAvg)
	}
	return nil
}
