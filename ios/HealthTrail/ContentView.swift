import SwiftUI

struct ContentView: View {
    @ObservedObject var appState: AppState

    var body: some View {
        ZStack {
            Color("LaunchBackground")
                .ignoresSafeArea()

            if let url = appState.activeURL {
                if appState.mode == .remote {
                    VStack(spacing: 0) {
                        remoteTopBar
                        WebViewContainer(
                            appState: appState,
                            targetURL: url,
                            reloadToken: appState.webViewReloadToken
                        )
                        .ignoresSafeArea(edges: .bottom)
                    }
                } else {
                    WebViewContainer(
                        appState: appState,
                        targetURL: url,
                        reloadToken: appState.webViewReloadToken
                    )
                    .ignoresSafeArea()
                }
            }

            if appState.isStartingLocal {
                loadingOverlay
            } else if let error = appState.loadError {
                errorOverlay(message: error)
            }
        }
        .sheet(isPresented: $appState.showConnectionSettings) {
            ConnectionSettingsView(appState: appState)
        }
        .sheet(item: $appState.previewItem) { item in
            AttachmentPreviewSheet(item: item)
        }
        .sheet(item: $appState.downloadedFile) { file in
            ShareSheet(items: [file.url])
        }
    }

    private var remoteTopBar: some View {
        VStack(spacing: 0) {
            HStack(spacing: 8) {
                Image(systemName: "desktopcomputer")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(Color.accentColor)

                Text(appState.remoteHostDisplay.isEmpty
                    ? L10n.pick(zh: "已连接家用电脑", en: "Home Computer")
                    : L10n.pick(
                        zh: "已连接电脑 · \(appState.remoteHostDisplay)",
                        en: "Home Computer · \(appState.remoteHostDisplay)"
                    ))
                    .font(.footnote.weight(.medium))
                    .foregroundStyle(.primary)
                    .lineLimit(1)
                    .truncationMode(.middle)

                Spacer(minLength: 8)

                Button {
                    appState.showConnectionSettings = true
                } label: {
                    Text(L10n.pick(zh: "切换", en: "Switch"))
                        .font(.footnote.weight(.semibold))
                        .padding(.horizontal, 12)
                        .padding(.vertical, 6)
                        .background(Color.accentColor.opacity(0.14), in: Capsule())
                        .foregroundStyle(Color.accentColor)
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("remote-switch-button")
            }
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(Color("LaunchBackground").ignoresSafeArea(edges: .top))

            Divider()
        }
    }

    private var loadingOverlay: some View {
        VStack(spacing: 14) {
            ProgressView()
                .controlSize(.large)
                .tint(Color.accentColor)
            Text(L10n.pick(zh: "正在启动本地健康档案…", en: "Starting Health Trail…"))
                .font(.subheadline)
                .foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color("LaunchBackground").ignoresSafeArea())
    }

    private func errorOverlay(message: String) -> some View {
        VStack(spacing: 20) {
            Spacer()

            Image(systemName: appState.mode == .remote ? "wifi.exclamationmark" : "exclamationmark.triangle")
                .font(.system(size: 44))
                .foregroundStyle(Color.accentColor)

            VStack(spacing: 8) {
                Text(appState.mode == .remote
                    ? L10n.pick(zh: "无法连接家用电脑", en: "Cannot Connect to Home Computer")
                    : L10n.pick(zh: "无法启动健康足迹", en: "Unable to Start Health Trail"))
                    .font(.title3.weight(.semibold))
                    .multilineTextAlignment(.center)

                Text(message)
                    .font(.subheadline)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 24)
            }

            VStack(spacing: 12) {
                Button {
                    Task { await appState.retryCurrentConnection() }
                } label: {
                    Label(L10n.pick(zh: "重试", en: "Retry"), systemImage: "arrow.clockwise")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.borderedProminent)
                .controlSize(.large)

                Button {
                    appState.showConnectionSettings = true
                } label: {
                    Label(L10n.pick(zh: "连接设置", en: "Connection Settings"), systemImage: "slider.horizontal.3")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.bordered)
                .controlSize(.large)

                if appState.mode == .remote {
                    Button {
                        Task { await appState.switchToLocal() }
                    } label: {
                        Label(L10n.pick(zh: "切换到本机模式（这台 iPhone）", en: "Switch to This iPhone (Local)"), systemImage: "iphone")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.borderless)
                    .controlSize(.large)
                }
            }
            .padding(.horizontal, 32)
            .padding(.top, 8)

            Spacer()
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color("LaunchBackground").ignoresSafeArea())
    }
}
