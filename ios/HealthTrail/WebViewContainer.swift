import SwiftUI
import UIKit
import WebKit

/// Weak wrapper to prevent WKUserContentController -> Coordinator retain cycles.
private final class WeakScriptMessageHandler: NSObject, WKScriptMessageHandler {
    weak var delegate: WKScriptMessageHandler?

    init(_ delegate: WKScriptMessageHandler) {
        self.delegate = delegate
    }

    func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
        delegate?.userContentController(userContentController, didReceive: message)
    }
}

struct WebViewContainer: UIViewRepresentable {
    @ObservedObject var appState: AppState
    let targetURL: URL
    let reloadToken: UUID

    func makeCoordinator() -> Coordinator {
        Coordinator(appState: appState)
    }

    func makeUIView(context: Context) -> WKWebView {
        let config = WKWebViewConfiguration()
        config.allowsInlineMediaPlayback = true
        config.defaultWebpagePreferences.allowsContentJavaScript = true

        let contentController = WKUserContentController()
        let proxy = WeakScriptMessageHandler(context.coordinator)
        contentController.add(proxy, name: "healthTrail")

        let script = WKUserScript(
            source: appState.injectedBridgeScript,
            injectionTime: .atDocumentStart,
            forMainFrameOnly: true
        )
        contentController.addUserScript(script)
        config.userContentController = contentController

        let webView = WKWebView(frame: .zero, configuration: config)
        webView.navigationDelegate = context.coordinator
        webView.uiDelegate = context.coordinator
        webView.isOpaque = false
        webView.backgroundColor = .clear
        webView.scrollView.backgroundColor = .clear
        // Frontend uses viewport-fit=cover + env(safe-area-inset-*), so disable automatic inset adjustment.
        webView.scrollView.contentInsetAdjustmentBehavior = .never
        webView.scrollView.keyboardDismissMode = .interactive
        webView.scrollView.alwaysBounceHorizontal = false
        webView.scrollView.showsHorizontalScrollIndicator = false
        webView.scrollView.delegate = context.coordinator
        // Use a custom left-edge swipe recognizer synchronized with the SPA's back stack
        // instead of WKWebView's raw history swipe (which clashes with hash routes & drawers).
        webView.allowsBackForwardNavigationGestures = false

        let edgeBack = UIScreenEdgePanGestureRecognizer(
            target: context.coordinator,
            action: #selector(Coordinator.handleEdgeSwipeBack(_:))
        )
        edgeBack.edges = .left
        edgeBack.delegate = context.coordinator
        webView.addGestureRecognizer(edgeBack)

        #if DEBUG
        if #available(iOS 16.4, *) {
            webView.isInspectable = true
        }
        #endif

        context.coordinator.webView = webView
        context.coordinator.lastLoadedURL = targetURL
        context.coordinator.lastReloadToken = reloadToken
        context.coordinator.lastBridgeScript = appState.injectedBridgeScript
        context.coordinator.bindSpeechEvents()

        context.coordinator.loadTargetURL(targetURL, in: webView)
        return webView
    }

    func updateUIView(_ webView: WKWebView, context: Context) {
        let currentScript = appState.injectedBridgeScript
        if context.coordinator.lastBridgeScript != currentScript {
            context.coordinator.lastBridgeScript = currentScript
            let controller = webView.configuration.userContentController
            controller.removeAllUserScripts()
            controller.addUserScript(WKUserScript(
                source: currentScript,
                injectionTime: .atDocumentStart,
                forMainFrameOnly: true
            ))
            webView.evaluateJavaScript(currentScript, completionHandler: nil)
        }

        if context.coordinator.lastLoadedURL != targetURL || context.coordinator.lastReloadToken != reloadToken {
            context.coordinator.lastLoadedURL = targetURL
            context.coordinator.lastReloadToken = reloadToken
            context.coordinator.loadTargetURL(targetURL, in: webView)
        }
    }

    static func dismantleUIView(_ uiView: WKWebView, coordinator: Coordinator) {
        Task { @MainActor in
            coordinator.appState.speechManager.stop()
            coordinator.appState.speechManager.onEvent = nil
        }
        uiView.configuration.userContentController.removeScriptMessageHandler(forName: "healthTrail")
        uiView.navigationDelegate = nil
        uiView.uiDelegate = nil
        uiView.scrollView.delegate = nil
    }

    final class Coordinator: NSObject, WKScriptMessageHandler, WKNavigationDelegate, WKUIDelegate, WKDownloadDelegate, UIScrollViewDelegate, UIGestureRecognizerDelegate {
        let appState: AppState
        weak var webView: WKWebView?
        var lastLoadedURL: URL?
        var lastReloadToken: UUID?
        var lastBridgeScript: String?
        private var pendingDownloadDestination: URL?

        init(appState: AppState) {
            self.appState = appState
        }

        // MARK: - UIScrollViewDelegate (prevent accidental pinch-zoom on main SPA)

        func viewForZooming(in scrollView: UIScrollView) -> UIView? {
            return nil
        }

        func scrollViewWillBeginZooming(_ scrollView: UIScrollView, with view: UIView?) {
            scrollView.pinchGestureRecognizer?.isEnabled = false
        }

        // MARK: - Left-Edge Swipe Back

        func gestureRecognizer(
            _ gestureRecognizer: UIGestureRecognizer,
            shouldRecognizeSimultaneouslyWith otherGestureRecognizer: UIGestureRecognizer
        ) -> Bool {
            return true
        }

        @objc func handleEdgeSwipeBack(_ recognizer: UIScreenEdgePanGestureRecognizer) {
            guard recognizer.state == .ended, let webView else { return }
            let translation = recognizer.translation(in: webView)
            let velocity = recognizer.velocity(in: webView)
            guard translation.x > 60 || velocity.x > 400 else { return }

            let js = """
            (function() {
              if (typeof window.__HEALTH_TRAIL_GO_BACK__ === 'function') {
                return window.__HEALTH_TRAIL_GO_BACK__() ? 'handled' : 'root';
              }
              return 'unavailable';
            })();
            """
            webView.evaluateJavaScript(js) { result, _ in
                guard let status = result as? String else { return }
                if status == "handled" {
                    let generator = UIImpactFeedbackGenerator(style: .light)
                    generator.impactOccurred()
                } else if status == "unavailable", webView.canGoBack {
                    webView.goBack()
                }
            }
        }

        // MARK: - Speech Bridge

        func bindSpeechEvents() {
            Task { @MainActor [weak self] in
                guard let self else { return }
                self.appState.speechManager.onEvent = { [weak self] event in
                    guard let self, let webView = self.webView else { return }
                    guard let data = try? JSONEncoder().encode(event),
                          let json = String(data: data, encoding: .utf8) else { return }
                    let js = "window.__HEALTH_TRAIL_ON_SPEECH__ && window.__HEALTH_TRAIL_ON_SPEECH__(\(json));"
                    Task { @MainActor in
                        _ = try? await webView.evaluateJavaScript(js)
                    }
                }
            }
        }

        // MARK: - WKScriptMessageHandler

        func userContentController(_ userContentController: WKUserContentController, didReceive message: WKScriptMessage) {
            guard message.name == "healthTrail" else { return }
            let dict = message.body as? [String: Any]
            let action: String?
            if let dict {
                action = dict["action"] as? String
            } else if let str = message.body as? String {
                action = str
            } else {
                action = nil
            }

            switch action {
            case "openConnectionSettings":
                Task { @MainActor in
                    self.appState.showConnectionSettings = true
                }
            case "getConnectionState":
                Task { @MainActor in
                    _ = try? await self.webView?.evaluateJavaScript(self.appState.injectedBridgeScript)
                }
            case "startSpeech":
                let lang = dict?["lang"] as? String
                Task { @MainActor in
                    self.appState.speechManager.start(lang: lang)
                }
            case "stopSpeech":
                Task { @MainActor in
                    self.appState.speechManager.stop()
                }
            default:
                break
            }
        }

        // MARK: - Remote Auth Cookie Injection

        func loadTargetURL(_ url: URL, in webView: WKWebView) {
            Task { @MainActor in
                let mode = self.appState.mode
                let token = self.appState.remoteToken.trimmingCharacters(in: .whitespacesAndNewlines)

                if mode == .remote, !token.isEmpty, let host = url.host {
                    var props: [HTTPCookiePropertyKey: Any] = [
                        .name: "hv_token",
                        .value: token,
                        .domain: host,
                        .path: "/",
                        .expires: Date().addingTimeInterval(365 * 24 * 3600),
                    ]
                    if url.scheme?.lowercased() == "https" {
                        props[.secure] = "TRUE"
                    }
                    if let cookie = HTTPCookie(properties: props) {
                        await webView.configuration.websiteDataStore.httpCookieStore.setCookie(cookie)
                    }
                }
                webView.load(URLRequest(url: url))
            }
        }

        // MARK: - WKNavigationDelegate

        private static func effectivePort(for url: URL) -> Int? {
            if let port = url.port { return port }
            switch url.scheme?.lowercased() {
            case "http": return 80
            case "https": return 443
            default: return nil
            }
        }

        private static func isSameOrigin(_ a: URL, _ b: URL) -> Bool {
            guard let schemeA = a.scheme?.lowercased(),
                  let schemeB = b.scheme?.lowercased(),
                  schemeA == schemeB,
                  let hostA = a.host?.lowercased(),
                  let hostB = b.host?.lowercased(),
                  hostA == hostB else {
                return false
            }
            return effectivePort(for: a) == effectivePort(for: b)
        }

        func webView(
            _ webView: WKWebView,
            decidePolicyFor navigationAction: WKNavigationAction,
            decisionHandler: @escaping (WKNavigationActionPolicy) -> Void
        ) {
            guard let url = navigationAction.request.url else {
                decisionHandler(.cancel)
                return
            }

            let isMainFrame = navigationAction.targetFrame?.isMainFrame ?? true
            let scheme = url.scheme?.lowercased() ?? ""

            // Allow about:blank (initial WKWebView state).
            if url.absoluteString == "about:blank" {
                decisionHandler(.allow)
                return
            }

            // Strict origin whitelist: only the active target origin (scheme + host + port) may load in the WebView.
            if let expected = lastLoadedURL, Self.isSameOrigin(url, expected) {
                decisionHandler(.allow)
                return
            }

            // User-activated external links open in Safari; never load third-party pages inside the privileged WebView.
            if isMainFrame,
               navigationAction.navigationType == .linkActivated,
               (scheme == "http" || scheme == "https" || scheme == "mailto" || scheme == "tel") {
                UIApplication.shared.open(url)
                decisionHandler(.cancel)
                return
            }

            // Block arbitrary data: HTML, cross-origin redirects, script navigations, and form submissions.
            decisionHandler(.cancel)
        }

        func webView(
            _ webView: WKWebView,
            decidePolicyFor navigationResponse: WKNavigationResponse,
            decisionHandler: @escaping (WKNavigationResponsePolicy) -> Void
        ) {
            if let http = navigationResponse.response as? HTTPURLResponse {
                // Detect Unauthorized on main-frame remote navigation.
                if navigationResponse.isForMainFrame, http.statusCode == 401 {
                    Task { @MainActor in
                        if self.appState.mode == .remote {
                            self.appState.loadError = L10n.pick(
                                zh: "家用电脑拒绝访问（401 未授权），请检查连接设置中的访问令牌。",
                                en: "Home Computer rejected the request (401 Unauthorized). Please check your access token in Connection Settings."
                            )
                        }
                    }
                    decisionHandler(.cancel)
                    return
                }

                // Trigger WKDownload for attachment downloads (e.g., backup .zip or non-previewable files).
                let disposition = http.value(forHTTPHeaderField: "Content-Disposition")?.lowercased() ?? ""
                if disposition.contains("attachment") || !navigationResponse.canShowMIMEType {
                    decisionHandler(.download)
                    return
                }
            } else if !navigationResponse.canShowMIMEType {
                decisionHandler(.download)
                return
            }

            decisionHandler(.allow)
        }

        func webView(_ webView: WKWebView, navigationResponse: WKNavigationResponse, didBecome download: WKDownload) {
            download.delegate = self
        }

        func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
            Task { @MainActor in
                self.appState.loadError = nil
            }
        }

        func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!, withError error: Error) {
            handleNavigationError(error)
        }

        func webView(_ webView: WKWebView, didFail navigation: WKNavigation!, withError error: Error) {
            handleNavigationError(error)
        }

        private func handleNavigationError(_ error: Error) {
            let nsError = error as NSError
            if nsError.domain == NSURLErrorDomain && nsError.code == NSURLErrorCancelled {
                return
            }
            // WebKitFrameLoadInterruptedByPolicyChange (102) happens on download handoff — not an error.
            if nsError.domain == "WebKitErrorDomain" && nsError.code == 102 {
                return
            }

            Task { @MainActor in
                if nsError.domain == NSURLErrorDomain,
                   nsError.code == URLError.appTransportSecurityRequiresSecureConnection.rawValue {
                    self.appState.loadError = AppState.atsBlockedMessage
                } else if self.appState.mode == .remote {
                    self.appState.loadError = L10n.pick(
                        zh: "无法连接到家用电脑（\(error.localizedDescription)）。您可以重试、修改连接设置，或切回本机模式离线使用。",
                        en: "Cannot connect to Home Computer (\(error.localizedDescription)). You can retry, update Connection Settings, or switch back to This iPhone."
                    )
                } else {
                    self.appState.loadError = error.localizedDescription
                }
            }
        }

        // MARK: - WKDownloadDelegate

        func download(
            _ download: WKDownload,
            decideDestinationUsing response: URLResponse,
            suggestedFilename: String,
            completionHandler: @escaping (URL?) -> Void
        ) {
            let tempDir = FileManager.default.temporaryDirectory
                .appendingPathComponent("HealthTrailDownloads", isDirectory: true)
            try? FileManager.default.createDirectory(at: tempDir, withIntermediateDirectories: true)
            // Sanitize filename to prevent path traversal outside tempDir.
            let rawComponent = (suggestedFilename as NSString).lastPathComponent
                .trimmingCharacters(in: .whitespacesAndNewlines)
            let sanitized = rawComponent
                .replacingOccurrences(of: "/", with: "_")
                .replacingOccurrences(of: "\\", with: "_")
            let safeName = (sanitized.isEmpty || sanitized == "." || sanitized == "..") ? "download" : sanitized
            let dest = tempDir.appendingPathComponent(safeName)
            try? FileManager.default.removeItem(at: dest)
            pendingDownloadDestination = dest
            completionHandler(dest)
        }

        func downloadDidFinish(_ download: WKDownload) {
            guard let url = pendingDownloadDestination else { return }
            pendingDownloadDestination = nil
            Task { @MainActor in
                self.appState.downloadedFile = DownloadedFileItem(url: url)
            }
        }

        func download(_ download: WKDownload, didFailWithError error: Error, resumeData: Data?) {
            pendingDownloadDestination = nil
        }

        // MARK: - WKUIDelegate

        func webView(
            _ webView: WKWebView,
            createWebViewWith configuration: WKWebViewConfiguration,
            for navigationAction: WKNavigationAction,
            windowFeatures: WKWindowFeatures
        ) -> WKWebView? {
            guard let url = navigationAction.request.url else { return nil }
            let isSameOrigin = lastLoadedURL.map { Self.isSameOrigin(url, $0) } ?? false

            if isSameOrigin {
                // Present PDF/image attachment in an in-app preview sheet so user keeps scroll state.
                let title = url.lastPathComponent.removingPercentEncoding ?? L10n.pick(zh: "附件预览", en: "Preview")
                Task { @MainActor in
                    self.appState.previewItem = PreviewItem(url: url, title: title)
                }
            } else if let scheme = url.scheme?.lowercased(), scheme == "http" || scheme == "https" {
                UIApplication.shared.open(url)
            }
            return nil
        }

        func webView(
            _ webView: WKWebView,
            runJavaScriptAlertPanelWithMessage message: String,
            initiatedByFrame frame: WKFrameInfo,
            completionHandler: @escaping () -> Void
        ) {
            presentAlert(title: nil, message: message, actions: [
                UIAlertAction(title: L10n.pick(zh: "好", en: "OK"), style: .default) { _ in completionHandler() }
            ], fallback: completionHandler)
        }

        func webView(
            _ webView: WKWebView,
            runJavaScriptConfirmPanelWithMessage message: String,
            initiatedByFrame frame: WKFrameInfo,
            completionHandler: @escaping (Bool) -> Void
        ) {
            presentAlert(title: nil, message: message, actions: [
                UIAlertAction(title: L10n.pick(zh: "取消", en: "Cancel"), style: .cancel) { _ in completionHandler(false) },
                UIAlertAction(title: L10n.pick(zh: "确定", en: "OK"), style: .default) { _ in completionHandler(true) },
            ], fallback: { completionHandler(false) })
        }

        func webView(
            _ webView: WKWebView,
            runJavaScriptTextInputPanelWithPrompt prompt: String,
            defaultText: String?,
            initiatedByFrame frame: WKFrameInfo,
            completionHandler: @escaping (String?) -> Void
        ) {
            guard let presenter = Self.topViewController() else {
                completionHandler(nil)
                return
            }
            let alert = UIAlertController(title: nil, message: prompt, preferredStyle: .alert)
            alert.addTextField { tf in
                tf.text = defaultText
            }
            alert.addAction(UIAlertAction(title: L10n.pick(zh: "取消", en: "Cancel"), style: .cancel) { _ in
                completionHandler(nil)
            })
            alert.addAction(UIAlertAction(title: L10n.pick(zh: "确定", en: "OK"), style: .default) { _ in
                completionHandler(alert.textFields?.first?.text)
            })
            presenter.present(alert, animated: true)
        }

        private func presentAlert(title: String?, message: String, actions: [UIAlertAction], fallback: () -> Void) {
            guard let presenter = Self.topViewController() else {
                fallback()
                return
            }
            let alert = UIAlertController(title: title, message: message, preferredStyle: .alert)
            actions.forEach { alert.addAction($0) }
            presenter.present(alert, animated: true)
        }

        private static func topViewController() -> UIViewController? {
            let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
            let keyWindow = scenes.flatMap { $0.windows }.first(where: { $0.isKeyWindow })
            var top = keyWindow?.rootViewController
            while let presented = top?.presentedViewController {
                top = presented
            }
            return top
        }
    }
}

/// Simple in-app preview for same-origin PDF / image attachments opened in a new window.
struct AttachmentPreviewSheet: View {
    let item: PreviewItem
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        NavigationStack {
            SimplePreviewWebView(url: item.url)
                .ignoresSafeArea(edges: .bottom)
                .navigationTitle(item.title)
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button(L10n.pick(zh: "完成", en: "Done")) {
                            dismiss()
                        }
                    }
                    ToolbarItem(placement: .primaryAction) {
                        ShareLink(item: item.url)
                    }
                }
        }
    }
}

private struct SimplePreviewWebView: UIViewRepresentable {
    let url: URL

    func makeUIView(context: Context) -> WKWebView {
        let webView = WKWebView(frame: .zero)
        webView.load(URLRequest(url: url))
        return webView
    }

    func updateUIView(_ uiView: WKWebView, context: Context) {}
}

struct ShareSheet: UIViewControllerRepresentable {
    let items: [Any]

    func makeUIViewController(context: Context) -> UIActivityViewController {
        UIActivityViewController(activityItems: items, applicationActivities: nil)
    }

    func updateUIViewController(_ uiViewController: UIActivityViewController, context: Context) {}
}
