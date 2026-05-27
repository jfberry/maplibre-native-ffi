// OpenGL offscreen-FBO render session. Mirrors the structure of
// opengl_texture_session.cpp but swaps the texture color attachment for a
// renderbuffer, matching mbgl's HeadlessBackend
// (platform/default/src/mbgl/gl/headless_backend.cpp). The texture-backed
// session's swap()=finish() and ContextMode::Shared cost ~5 ms p50 on Mesa
// llvmpipe because they force GPU sync per frame; this backend uses the same
// no-flush swap + Unique context as HeadlessBackend, so per-frame latency is
// at parity with the Node binding's mbgl HeadlessBackend path.
//
// The EGL/WGL context-management boilerplate is duplicated from
// opengl_texture_session.cpp deliberately — keeping the two backends in
// separate translation units makes both easier to reason about and lets the
// offscreen path evolve independently of the texture-frame API.

#include <cstddef>
#include <cstdint>
#include <memory>
#include <optional>
#include <stdexcept>
#include <string>
#include <utility>

#include <mbgl/gfx/backend_scope.hpp>
#include <mbgl/gfx/headless_backend.hpp>
#include <mbgl/gfx/renderable.hpp>
#include <mbgl/gl/context.hpp>
#include <mbgl/gl/defines.hpp>
#include <mbgl/gl/framebuffer.hpp>
#include <mbgl/gl/renderable_resource.hpp>
#include <mbgl/gl/renderbuffer_resource.hpp>
#include <mbgl/gl/renderer_backend.hpp>
#include <mbgl/platform/gl_functions.hpp>
#include <mbgl/util/image.hpp>
#include <mbgl/util/size.hpp>

#if defined(_WIN32)
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN 1
#endif
#include <Windows.h>
#elif defined(__linux__)
#include <EGL/egl.h>
#endif

#include "diagnostics/diagnostics.hpp"
#include "map/map.hpp"
#include "maplibre_native_c/base.h"
#if defined(__linux__)
#include "render/opengl/egl_common.hpp"
#endif
#include "render/opengl/wgl_common.hpp"
#include "render/render_session_common.hpp"
#include "render/texture_session.hpp"

namespace {

auto validate_opengl_offscreen_descriptor(
  const mln_opengl_offscreen_descriptor* descriptor
) -> mln_status {
  if (descriptor == nullptr) {
    mln::core::set_thread_error("offscreen descriptor must not be null");
    return MLN_STATUS_INVALID_ARGUMENT;
  }
  if (descriptor->size < sizeof(mln_opengl_offscreen_descriptor)) {
    mln::core::set_thread_error(
      "mln_opengl_offscreen_descriptor.size is too small"
    );
    return MLN_STATUS_INVALID_ARGUMENT;
  }
  const auto extent_status = mln::core::validate_render_target_extent(
    descriptor->extent, "offscreen dimensions and scale_factor must be positive"
  );
  if (extent_status != MLN_STATUS_OK) {
    return extent_status;
  }
  return mln::core::validate_opengl_context(descriptor->context, true);
}

class OpenGLOffscreenRenderableResource final
    : public mbgl::gl::RenderableResource {
 public:
  OpenGLOffscreenRenderableResource(
    mbgl::gl::Context& context_, mbgl::Size size_
  )
      : context(context_), size(size_) {}

  ~OpenGLOffscreenRenderableResource() noexcept override = default;

  void bind() override {
    try {
      ensure_resources();
      context.bindFramebuffer = framebuffer_->framebuffer;
      context.scissorTest = {0, 0, 0, 0};
      context.viewport = {0, 0, size};
    } catch (const std::exception& exception) {
      throw std::runtime_error(
        std::string{"binding OpenGL offscreen renderable: "} + exception.what()
      );
    }
  }

  // Match mbgl HeadlessBackend (no-flush). glReadPixels is already a
  // synchronous read on the bound framebuffer; an explicit finish here
  // would only serialise renders unnecessarily.
  void swap() override {}

  auto readStillImage() -> mbgl::PremultipliedImage {
    bind();
    return context.readFramebuffer<mbgl::PremultipliedImage>(size);
  }

 private:
  void ensure_resources() {
    if (framebuffer_) {
      return;
    }

    color_ =
      context.createRenderbuffer<mbgl::gfx::RenderbufferPixelType::RGBA>(size);
    depth_stencil_ =
      context
        .createRenderbuffer<mbgl::gfx::RenderbufferPixelType::DepthStencil>(
          size
        );
    framebuffer_ = context.createFramebuffer(*color_, *depth_stencil_);
  }

  mbgl::gl::Context& context;
  mbgl::Size size;
  std::optional<mbgl::gfx::Renderbuffer<mbgl::gfx::RenderbufferPixelType::RGBA>>
    color_;
  std::optional<
    mbgl::gfx::Renderbuffer<mbgl::gfx::RenderbufferPixelType::DepthStencil>>
    depth_stencil_;
  std::optional<mbgl::gl::Framebuffer> framebuffer_;
};

class OpenGLOffscreenBackend final : public mbgl::gl::RendererBackend,
                                     public mbgl::gfx::HeadlessBackend {
 public:
  OpenGLOffscreenBackend(
    const mln_opengl_offscreen_descriptor& descriptor, mbgl::Size size
  )
      : mbgl::gl::RendererBackend(mbgl::gfx::ContextMode::Unique),
        mbgl::gfx::HeadlessBackend(size),
        context_(descriptor.context) {}

  OpenGLOffscreenBackend(const OpenGLOffscreenBackend&) = delete;
  auto operator=(const OpenGLOffscreenBackend&)
    -> OpenGLOffscreenBackend& = delete;
  OpenGLOffscreenBackend(OpenGLOffscreenBackend&&) = delete;
  auto operator=(OpenGLOffscreenBackend&&) -> OpenGLOffscreenBackend& = delete;

  ~OpenGLOffscreenBackend() override {
    auto cleanup = [this] {
      resource.reset();
      context.reset();
    };
    if (render_context_ != nullptr) {
      auto guard = mbgl::gfx::BackendScope{
        *this, mbgl::gfx::BackendScope::ScopeType::Implicit
      };
      cleanup();
    } else {
      cleanup();
    }
    getThreadPool().runRenderJobs(true);
    destroy_native_context();
  }

  auto getDefaultRenderable() -> mbgl::gfx::Renderable& override {
    const auto current_size = getSize();
    if (!resource || resource_size_ != current_size) {
      resource = std::make_unique<OpenGLOffscreenRenderableResource>(
        getContext<mbgl::gl::Context>(), current_size
      );
      resource_size_ = current_size;
    }
    return *this;
  }

  auto readStillImage() -> mbgl::PremultipliedImage override {
    auto& renderable =
      getDefaultRenderable().getResource<OpenGLOffscreenRenderableResource>();
    return renderable.readStillImage();
  }

  auto getRendererBackend() -> mbgl::gfx::RendererBackend* override {
    return this;
  }

  void updateAssumedState() override {
    assumeFramebufferBinding(
      mbgl::gl::RendererBackend::ImplicitFramebufferBinding
    );
  }

 private:
  auto getExtensionFunctionPointer(const char* name)
    -> mbgl::gl::ProcAddress override {
#if defined(_WIN32)
    using GetProcAddressFunction = PROC(WINAPI*)(LPCSTR);
    auto* loader = reinterpret_cast<GetProcAddressFunction>(
      context_.data.wgl.get_proc_address
    );
    if (loader != nullptr) {
      auto* proc = loader(name);
      if (mln::core::opengl::is_valid_wgl_proc_address(proc)) {
        return reinterpret_cast<mbgl::gl::ProcAddress>(proc);
      }
    }
    auto* proc = wglGetProcAddress(name);
    if (mln::core::opengl::is_valid_wgl_proc_address(proc)) {
      return reinterpret_cast<mbgl::gl::ProcAddress>(proc);
    }
    return reinterpret_cast<mbgl::gl::ProcAddress>(
      mln::core::opengl::get_wgl_client_library_proc_address(name)
    );
#elif defined(__linux__)
    using GetProcAddressFunction = void* (*)(const char*);
    auto* loader = reinterpret_cast<GetProcAddressFunction>(
      context_.data.egl.get_proc_address
    );
    if (loader != nullptr) {
      auto* proc = loader(name);
      if (proc != nullptr) {
        return reinterpret_cast<mbgl::gl::ProcAddress>(proc);
      }
    }
    if (auto* proc = eglGetProcAddress(name); proc != nullptr) {
      return reinterpret_cast<mbgl::gl::ProcAddress>(proc);
    }
    return reinterpret_cast<mbgl::gl::ProcAddress>(
      mln::core::opengl::get_egl_client_library_proc_address(name, active_api_)
    );
#else
    (void)name;
    return nullptr;
#endif
  }

  void activate() override {
#if defined(_WIN32)
    previous_device_context_ = wglGetCurrentDC();
    previous_render_context_ = wglGetCurrentContext();
    try {
      if (render_context_ == nullptr) {
        create_wgl_context();
      }
      if (
        wglMakeCurrent(
          static_cast<HDC>(context_.data.wgl.device_context),
          static_cast<HGLRC>(render_context_)
        ) == 0
      ) {
        throw std::runtime_error("Switching OpenGL WGL context failed");
      }
      validate_wgl_context_support();
    } catch (...) {
      (void)wglMakeCurrent(
        static_cast<HDC>(previous_device_context_),
        static_cast<HGLRC>(previous_render_context_)
      );
      previous_device_context_ = nullptr;
      previous_render_context_ = nullptr;
      throw;
    }
#elif defined(__linux__)
    previous_display_ = eglGetCurrentDisplay();
    previous_draw_surface_ = eglGetCurrentSurface(EGL_DRAW);
    previous_read_surface_ = eglGetCurrentSurface(EGL_READ);
    previous_context_ = eglGetCurrentContext();
    previous_api_ = eglQueryAPI();
    try {
      const auto requested_api = share_context_api();
      if (eglBindAPI(requested_api) == EGL_FALSE) {
        throw std::runtime_error("Binding EGL OpenGL API failed");
      }
      active_api_ = requested_api;
      if (render_context_ == nullptr) {
        create_egl_context();
      }
      if (
        eglMakeCurrent(
          static_cast<EGLDisplay>(context_.data.egl.display),
          static_cast<EGLSurface>(surface_), static_cast<EGLSurface>(surface_),
          static_cast<EGLContext>(render_context_)
        ) == EGL_FALSE
      ) {
        throw std::runtime_error("Switching OpenGL EGL context failed");
      }
    } catch (...) {
      if (active_api_ != EGL_NONE) {
        release_current_egl_context();
      }
      restore_previous_egl_api();
      restore_previous_egl_context();
      throw;
    }
#else
    throw std::runtime_error("OpenGL context provider is unsupported");
#endif
  }

  void deactivate() override {
#if defined(_WIN32)
    wglMakeCurrent(
      static_cast<HDC>(previous_device_context_),
      static_cast<HGLRC>(previous_render_context_)
    );
    previous_device_context_ = nullptr;
    previous_render_context_ = nullptr;
#elif defined(__linux__)
    release_current_egl_context();
    restore_previous_egl_api();
    restore_previous_egl_context();
#endif
  }

#if defined(_WIN32)
  void create_wgl_context() {
    auto* const device_context =
      static_cast<HDC>(context_.data.wgl.device_context);
    auto* const share_context =
      static_cast<HGLRC>(context_.data.wgl.share_context);
    auto* context_attribs =
      reinterpret_cast<mln::core::opengl::WglCreateContextAttribs>(
        getExtensionFunctionPointer("wglCreateContextAttribsARB")
      );
    render_context_ = mln::core::opengl::create_shared_wgl_context(
      device_context, share_context,
      static_cast<HGLRC>(previous_render_context_), context_attribs
    );
  }

  void validate_wgl_context_support() {
    mln::core::opengl::validate_required_wgl_proc_addresses(
      [this](const char* name) { return getExtensionFunctionPointer(name); }
    );
  }

  void destroy_native_context() {
    if (render_context_ != nullptr) {
      wglDeleteContext(static_cast<HGLRC>(render_context_));
      render_context_ = nullptr;
    }
  }
#elif defined(__linux__)
  void create_egl_context() {
    auto* const display = static_cast<EGLDisplay>(context_.data.egl.display);
    auto* const config = static_cast<EGLConfig>(context_.data.egl.config);
    auto* const share_context =
      static_cast<EGLContext>(context_.data.egl.share_context);

    const EGLint es_context_attributes[] = {
      EGL_CONTEXT_CLIENT_VERSION, 3, EGL_NONE
    };
    const EGLint opengl_context_attributes[] = {EGL_NONE};
    auto* const context_attributes = active_api_ == EGL_OPENGL_ES_API
                                       ? es_context_attributes
                                       : opengl_context_attributes;
    render_context_ =
      eglCreateContext(display, config, share_context, context_attributes);
    if (render_context_ == EGL_NO_CONTEXT) {
      render_context_ = nullptr;
      throw std::runtime_error("Creating OpenGL EGL context failed");
    }

    const EGLint surface_attributes[] = {
      EGL_WIDTH, 8, EGL_HEIGHT, 8, EGL_LARGEST_PBUFFER, EGL_TRUE, EGL_NONE
    };
    surface_ = eglCreatePbufferSurface(display, config, surface_attributes);
    if (surface_ == EGL_NO_SURFACE) {
      eglDestroyContext(display, static_cast<EGLContext>(render_context_));
      render_context_ = nullptr;
      surface_ = nullptr;
      throw std::runtime_error("Creating OpenGL EGL pbuffer failed");
    }
  }

  auto share_context_api() -> EGLenum {
    auto* const display = static_cast<EGLDisplay>(context_.data.egl.display);
    auto* const share_context =
      static_cast<EGLContext>(context_.data.egl.share_context);
    auto client_type = EGLint{};
    if (
      eglQueryContext(
        display, share_context, EGL_CONTEXT_CLIENT_TYPE, &client_type
      ) == EGL_FALSE
    ) {
      throw std::runtime_error("Querying OpenGL EGL context API failed");
    }
    if (client_type == EGL_OPENGL_API || client_type == EGL_OPENGL_ES_API) {
      return static_cast<EGLenum>(client_type);
    }
    throw std::runtime_error("OpenGL EGL context API is unsupported");
  }

  void release_current_egl_context() {
    auto* const display = static_cast<EGLDisplay>(context_.data.egl.display);
    (void)eglMakeCurrent(
      display, EGL_NO_SURFACE, EGL_NO_SURFACE, EGL_NO_CONTEXT
    );
  }

  void restore_previous_egl_api() {
    if (previous_api_ != EGL_NONE) {
      eglBindAPI(previous_api_);
      previous_api_ = EGL_NONE;
    }
    active_api_ = EGL_NONE;
  }

  void restore_previous_egl_context() {
    const auto had_previous_display = previous_display_ != nullptr;
    auto* const display =
      had_previous_display ? static_cast<EGLDisplay>(previous_display_)
                           : static_cast<EGLDisplay>(context_.data.egl.display);
    auto* const draw_surface =
      had_previous_display ? static_cast<EGLSurface>(previous_draw_surface_)
                           : EGL_NO_SURFACE;
    auto* const read_surface =
      had_previous_display ? static_cast<EGLSurface>(previous_read_surface_)
                           : EGL_NO_SURFACE;
    auto* const context = had_previous_display
                            ? static_cast<EGLContext>(previous_context_)
                            : EGL_NO_CONTEXT;
    (void)eglMakeCurrent(display, draw_surface, read_surface, context);
    previous_display_ = nullptr;
    previous_draw_surface_ = nullptr;
    previous_read_surface_ = nullptr;
    previous_context_ = nullptr;
  }

  void destroy_native_context() {
    auto* const display = static_cast<EGLDisplay>(context_.data.egl.display);
    if (surface_ != nullptr) {
      eglDestroySurface(display, static_cast<EGLSurface>(surface_));
      surface_ = nullptr;
    }
    if (render_context_ != nullptr) {
      eglDestroyContext(display, static_cast<EGLContext>(render_context_));
      render_context_ = nullptr;
    }
  }
#else
  void destroy_native_context() {}
#endif

  mln_opengl_context_descriptor context_{};
  void* render_context_ = nullptr;
  mbgl::Size resource_size_{};

#if defined(_WIN32)
  void* previous_device_context_ = nullptr;
  void* previous_render_context_ = nullptr;
#elif defined(__linux__)
  void* surface_ = nullptr;
  void* previous_display_ = nullptr;
  void* previous_draw_surface_ = nullptr;
  void* previous_read_surface_ = nullptr;
  void* previous_context_ = nullptr;
  EGLenum previous_api_ = EGL_NONE;
  EGLenum active_api_ = EGL_NONE;
#endif
};

class OpenGLOffscreenSessionBackend final
    : public mln::core::TextureSessionBackend {
 public:
  OpenGLOffscreenSessionBackend(
    const mln_opengl_offscreen_descriptor& descriptor, mbgl::Size size
  )
      : backend_(descriptor, size) {}

  auto headless_backend() -> mbgl::gfx::HeadlessBackend& override {
    return backend_;
  }

 private:
  OpenGLOffscreenBackend backend_;
};

}  // namespace

namespace mln::core {

auto opengl_offscreen_descriptor_default() noexcept
  -> mln_opengl_offscreen_descriptor {
  return mln_opengl_offscreen_descriptor{
    .size = sizeof(mln_opengl_offscreen_descriptor),
    .extent =
      mln_render_target_extent{
        .size = sizeof(mln_render_target_extent),
        .width = 256,
        .height = 256,
        .scale_factor = 1.0,
      },
    .context = opengl_context_descriptor_default(),
  };
}

auto opengl_offscreen_attach(
  mln_map* map, const mln_opengl_offscreen_descriptor* descriptor,
  mln_render_session** out_session
) -> mln_status {
  const auto map_status = validate_map(map);
  if (map_status != MLN_STATUS_OK) {
    return map_status;
  }
  const auto descriptor_status =
    validate_opengl_offscreen_descriptor(descriptor);
  if (descriptor_status != MLN_STATUS_OK) {
    return descriptor_status;
  }
  const auto output_status = validate_attach_output(
    out_session, "out_session must not be null",
    "out_session must point to a null handle"
  );
  if (output_status != MLN_STATUS_OK) {
    return output_status;
  }
  const auto physical_status = validate_physical_size(
    descriptor->extent.width, descriptor->extent.height,
    descriptor->extent.scale_factor, "scaled offscreen dimensions are too large"
  );
  if (physical_status != MLN_STATUS_OK) {
    return physical_status;
  }

  auto session = std::make_unique<mln_render_session>();
  session->map = map;
  session->owner_thread = map_owner_thread(map);
  set_session_extent(*session, descriptor->extent);
  // The offscreen session is conceptually a texture-kind session for the
  // purposes of texture_read_premultiplied_rgba8 (which dispatches on
  // RenderSessionKind::Texture), but it does not expose owned-frame APIs.
  session->texture.api_kind = TextureSessionApi::OpenGL;
  session->texture.mode = TextureSessionMode::Owned;
  session->texture.backend = std::make_unique<OpenGLOffscreenSessionBackend>(
    *descriptor, mbgl::Size{session->physical_width, session->physical_height}
  );
  return attach_render_session(
    std::move(session), out_session, RenderSessionKind::Texture,
    RenderSessionAttachMessages{
      .null_session = "offscreen session must not be null",
      .null_output = "out_session must not be null",
      .non_null_output = "out_session must point to a null handle"
    }
  );
}

}  // namespace mln::core
