// Synchronous MainResourceLoader replacement.
//
// mbgl::MainResourceLoader serialises every request through a dedicated
// util::Thread<MainResourceLoaderThread>, which costs one actor hop per
// request before the per-source FileSource is even invoked. For a render
// burst of ~25 resources (style + glyph/sprite/tile fan-out) that's ~25
// hops of pure scheduling latency before any I/O begins. The Node binding
// sidesteps this by registering its own FileSource as
// FileSourceType::ResourceLoader (see platform/node/src/node_map.cpp); we
// take the same shape: construct the per-scheme FileSources directly and
// run the waterfall from MainResourceLoaderThread::request inline on the
// caller thread.
//
// Caveat: the per-scheme FileSources (MBTilesFileSource, OnlineFileSource,
// etc.) still have their own internal actor/worker threads, so this only
// saves the *first* hop. That's enough for the spike — if the remaining
// 10 ms vs the Node binding really is in MainResourceLoader's dispatch
// queue, this closes it.

#include <cstdint>
#include <cstdlib>
#include <cstring>
#include <map>
#include <memory>
#include <mutex>
#include <utility>

#include <mbgl/storage/file_source.hpp>
#include <mbgl/storage/file_source_manager.hpp>
#include <mbgl/storage/file_source_request.hpp>
#include <mbgl/storage/resource.hpp>
#include <mbgl/storage/resource_options.hpp>
#include <mbgl/storage/response.hpp>
#include <mbgl/util/async_request.hpp>
#include <mbgl/util/client_options.hpp>

#include "resources/resource_loader.hpp"

namespace mln::core {
namespace {

// Read once. The env var lets consumers opt into the bypass without
// rebuilding. Default is the standard mbgl::MainResourceLoader (actor
// dispatch) — the local Scottish-walk bench showed the bypass did not close
// the remaining latency gap vs the Node binding, so the safer code path is
// kept as the default. Set MLN_FFI_RESOURCE_LOADER=bypass to switch.
auto loader_kind_is_bypass() noexcept -> bool {
  static const auto kind = [] {
    const auto* env = std::getenv("MLN_FFI_RESOURCE_LOADER");
    if (env == nullptr) {
      return false;  // default: standard mbgl::MainResourceLoader
    }
    return std::strcmp(env, "bypass") == 0;
  }();
  return kind;
}

class BypassResourceLoader final : public mbgl::FileSource {
 public:
  BypassResourceLoader(
    mbgl::ResourceOptions resource_options, mbgl::ClientOptions client_options
  )
      : asset_(
          mbgl::FileSourceManager::get()->getFileSource(
            mbgl::FileSourceType::Asset, resource_options, client_options
          )
        ),
        database_(
          mbgl::FileSourceManager::get()->getFileSource(
            mbgl::FileSourceType::Database, resource_options, client_options
          )
        ),
        local_(
          mbgl::FileSourceManager::get()->getFileSource(
            mbgl::FileSourceType::FileSystem, resource_options, client_options
          )
        ),
        online_(
          mbgl::FileSourceManager::get()->getFileSource(
            mbgl::FileSourceType::Network, resource_options, client_options
          )
        ),
        mbtiles_(
          mbgl::FileSourceManager::get()->getFileSource(
            mbgl::FileSourceType::Mbtiles, resource_options, client_options
          )
        ),
        pmtiles_(
          mbgl::FileSourceManager::get()->getFileSource(
            mbgl::FileSourceType::Pmtiles, resource_options, client_options
          )
        ),
        supports_cache_only_(static_cast<bool>(database_)),
        resource_options_(std::move(resource_options)),
        client_options_(std::move(client_options)) {}

  ~BypassResourceLoader() override = default;

  auto request(const mbgl::Resource& resource, Callback callback)
    -> std::unique_ptr<mbgl::AsyncRequest> override {
    auto req = std::make_unique<mbgl::FileSourceRequest>(std::move(callback));
    req->onCancel([this, key = req.get()] {
      auto lock = std::scoped_lock{tasks_mutex_};
      tasks_.erase(key);
    });
    dispatch(req->actor(), req.get(), resource);
    return req;
  }

  auto canRequest(const mbgl::Resource& resource) const -> bool override {
    return (asset_ && asset_->canRequest(resource)) ||
           (mbtiles_ && mbtiles_->canRequest(resource)) ||
           (pmtiles_ && pmtiles_->canRequest(resource)) ||
           (local_ && local_->canRequest(resource)) ||
           (database_ && database_->canRequest(resource)) ||
           (online_ && online_->canRequest(resource));
  }

  auto supportsCacheOnlyRequests() const -> bool override {
    return supports_cache_only_;
  }

  void pause() override {}
  void resume() override {}

  void setResourceTransform(mbgl::ResourceTransform transform) override {
    if (online_) {
      online_->setResourceTransform(std::move(transform));
    }
  }

  void setResourceOptions(mbgl::ResourceOptions options) override {
    auto lock = std::scoped_lock{options_mutex_};
    resource_options_ = options.clone();
    if (asset_) asset_->setResourceOptions(options.clone());
    if (database_) database_->setResourceOptions(options.clone());
    if (local_) local_->setResourceOptions(options.clone());
    if (online_) online_->setResourceOptions(options.clone());
    if (mbtiles_) mbtiles_->setResourceOptions(options.clone());
    if (pmtiles_) pmtiles_->setResourceOptions(options.clone());
  }

  auto getResourceOptions() -> mbgl::ResourceOptions override {
    auto lock = std::scoped_lock{options_mutex_};
    return resource_options_.clone();
  }

  void setClientOptions(mbgl::ClientOptions options) override {
    auto lock = std::scoped_lock{client_mutex_};
    client_options_ = options.clone();
    if (asset_) asset_->setClientOptions(options.clone());
    if (database_) database_->setClientOptions(options.clone());
    if (local_) local_->setClientOptions(options.clone());
    if (online_) online_->setClientOptions(options.clone());
    if (mbtiles_) mbtiles_->setClientOptions(options.clone());
    if (pmtiles_) pmtiles_->setClientOptions(options.clone());
  }

  auto getClientOptions() -> mbgl::ClientOptions override {
    auto lock = std::scoped_lock{client_mutex_};
    return client_options_.clone();
  }

 private:
  // Mirrors MainResourceLoaderThread::request (lines 33-126 of
  // mbgl/platform/default/src/mbgl/storage/main_resource_loader.cpp) but runs
  // on the caller thread instead of the ResourceLoaderThread actor.
  void dispatch(
    mbgl::ActorRef<mbgl::FileSourceRequest> ref, mbgl::FileSourceRequest* key,
    const mbgl::Resource& resource
  ) {
    auto deliver = [ref](const mbgl::Response& response) {
      ref.invoke(&mbgl::FileSourceRequest::setResponse, response);
    };

    auto from_network = [this, deliver](
                          const mbgl::Resource& res,
                          std::unique_ptr<mbgl::AsyncRequest> parent
                        ) -> std::unique_ptr<mbgl::AsyncRequest> {
      if (!online_ || !online_->canRequest(res)) {
        return parent;
      }
      auto parent_keep_alive =
        std::shared_ptr<mbgl::AsyncRequest>(std::move(parent));
      return online_->request(
        res, [this, deliver, res,
              parent_keep_alive](const mbgl::Response& response) {
          if (database_) {
            database_->forward(res, response, nullptr);
          }
          deliver(response);
        }
      );
    };

    auto store_task = [this, key](std::unique_ptr<mbgl::AsyncRequest> task) {
      if (task == nullptr) {
        return;
      }
      auto lock = std::scoped_lock{tasks_mutex_};
      tasks_[key] = std::move(task);
    };

    if (asset_ && asset_->canRequest(resource)) {
      store_task(asset_->request(resource, deliver));
    } else if (mbtiles_ && mbtiles_->canRequest(resource)) {
      store_task(mbtiles_->request(resource, deliver));
    } else if (pmtiles_ && pmtiles_->canRequest(resource)) {
      store_task(pmtiles_->request(resource, deliver));
    } else if (local_ && local_->canRequest(resource)) {
      store_task(local_->request(resource, deliver));
    } else if (database_ && database_->canRequest(resource)) {
      if (resource.loadingMethod == mbgl::Resource::LoadingMethod::CacheOnly) {
        store_task(database_->request(resource, deliver));
      } else {
        store_task(database_->request(
          resource, [this, key, deliver, resource,
                     from_network](const mbgl::Response& response) {
            auto res = resource;
            if (!response.noContent) {
              if (response.isUsable()) {
                deliver(response);
                res.setPriority(mbgl::Resource::Priority::Low);
              } else {
                res.priorData = response.data;
              }
              res.priorModified = response.modified;
              res.priorExpires = response.expires;
              res.priorEtag = response.etag;
            }
            auto chained = std::unique_ptr<mbgl::AsyncRequest>{};
            {
              auto lock = std::scoped_lock{tasks_mutex_};
              auto it = tasks_.find(key);
              if (it != tasks_.end()) {
                chained = std::move(it->second);
              }
            }
            chained = from_network(res, std::move(chained));
            if (chained) {
              auto lock = std::scoped_lock{tasks_mutex_};
              tasks_[key] = std::move(chained);
            }
          }
        ));
      }
    } else if (auto network_req = from_network(resource, nullptr)) {
      store_task(std::move(network_req));
    } else {
      mbgl::Response response;
      response.noContent = true;
      response.error = std::make_unique<mbgl::Response::Error>(
        mbgl::Response::Error::Reason::Other, "Unsupported resource request."
      );
      deliver(response);
    }
  }

  std::shared_ptr<mbgl::FileSource> asset_;
  std::shared_ptr<mbgl::FileSource> database_;
  std::shared_ptr<mbgl::FileSource> local_;
  std::shared_ptr<mbgl::FileSource> online_;
  std::shared_ptr<mbgl::FileSource> mbtiles_;
  std::shared_ptr<mbgl::FileSource> pmtiles_;
  bool supports_cache_only_;

  std::mutex tasks_mutex_;
  std::map<mbgl::FileSourceRequest*, std::unique_ptr<mbgl::AsyncRequest>>
    tasks_;

  std::mutex options_mutex_;
  mbgl::ResourceOptions resource_options_;
  std::mutex client_mutex_;
  mbgl::ClientOptions client_options_;
};

}  // namespace

auto make_bypass_resource_loader(
  const mbgl::ResourceOptions& resource_options,
  const mbgl::ClientOptions& client_options
) noexcept -> std::unique_ptr<mbgl::FileSource> {
  try {
    return std::make_unique<BypassResourceLoader>(
      resource_options.clone(), client_options.clone()
    );
  } catch (...) {
    return nullptr;
  }
}

auto bypass_resource_loader_enabled() noexcept -> bool {
  return loader_kind_is_bypass();
}

}  // namespace mln::core
