import SwiftUI

struct ConnectionSettingsView: View {
    @ObservedObject var appState: AppState
    @Environment(\.dismiss) private var dismiss

    @State private var selectedMode: ConnectionMode = .local
    @State private var serverURL: String = ""
    @State private var accessToken: String = ""
    @State private var showToken: Bool = false

    @State private var isTesting: Bool = false
    @State private var testStatusMessage: String?
    @State private var testSucceeded: Bool = false
    @State private var isSaving: Bool = false

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    modeRow(
                        mode: .local,
                        icon: "iphone",
                        title: L10n.pick(zh: "本机模式（这台 iPhone）", en: "This iPhone (Local)"),
                        subtitle: L10n.pick(
                            zh: "健康档案完全保存在这台 iPhone 上，离线可用，无需电脑。",
                            en: "Records stay privately on this iPhone. Works completely offline."
                        )
                    )
                    modeRow(
                        mode: .remote,
                        icon: "desktopcomputer",
                        title: L10n.pick(zh: "连接家用电脑", en: "Home Computer"),
                        subtitle: L10n.pick(
                            zh: "通过局域网 Wi-Fi 或 Tailscale 访问电脑上的健康足迹。",
                            en: "Connect to Health Trail running on your Mac or PC over Wi-Fi or Tailscale."
                        )
                    )
                } header: {
                    Text(L10n.pick(zh: "数据位置", en: "Data Source"))
                }

                if selectedMode == .remote {
                    Section {
                        VStack(alignment: .leading, spacing: 6) {
                            Text(L10n.pick(zh: "电脑地址", en: "Server URL"))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            TextField(
                                "http://192.168.1.10:8721",
                                text: $serverURL
                            )
                            .keyboardType(.URL)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .onChange(of: serverURL) { newValue in
                                handlePossiblePhoneLinkPaste(newValue)
                            }
                        }

                        VStack(alignment: .leading, spacing: 6) {
                            Text(L10n.pick(zh: "访问令牌", en: "Access Token"))
                                .font(.caption)
                                .foregroundStyle(.secondary)
                            HStack {
                                Group {
                                    if showToken {
                                        TextField(
                                            L10n.pick(zh: "粘贴电脑端的访问令牌", en: "Paste access token"),
                                            text: $accessToken
                                        )
                                    } else {
                                        SecureField(
                                            L10n.pick(zh: "粘贴电脑端的访问令牌", en: "Paste access token"),
                                            text: $accessToken
                                        )
                                    }
                                }
                                .textInputAutocapitalization(.never)
                                .autocorrectionDisabled()

                                Button {
                                    showToken.toggle()
                                } label: {
                                    Image(systemName: showToken ? "eye.slash" : "eye")
                                        .foregroundStyle(.secondary)
                                }
                                .buttonStyle(.plain)
                                .accessibilityLabel(showToken
                                    ? L10n.pick(zh: "隐藏令牌", en: "Hide token")
                                    : L10n.pick(zh: "显示令牌", en: "Show token"))
                            }
                        }

                        Button {
                            Task { await runConnectionTest() }
                        } label: {
                            HStack {
                                if isTesting {
                                    ProgressView()
                                        .controlSize(.small)
                                } else {
                                    Image(systemName: "network")
                                }
                                Text(L10n.pick(zh: "测试连接", en: "Test Connection"))
                            }
                        }
                        .disabled(isTesting || serverURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)

                        if let msg = testStatusMessage {
                            HStack(alignment: .top, spacing: 8) {
                                Image(systemName: testSucceeded ? "checkmark.circle.fill" : "exclamationmark.triangle.fill")
                                    .foregroundStyle(testSucceeded ? .green : .red)
                                    .padding(.top, 2)
                                Text(msg)
                                    .font(.footnote)
                                    .foregroundStyle(testSucceeded ? .green : .red)
                            }
                            .padding(.vertical, 2)
                        }
                    } header: {
                        Text(L10n.pick(zh: "家用电脑连接信息", en: "Home Computer Details"))
                    } footer: {
                        Text(L10n.pick(
                            zh: "局域网请填 http://192.168.x.x:8721；Tailscale 请使用 MagicDNS（http://电脑名.tailnet.ts.net:8721）或 HTTPS（tailscale serve），勿填明文 http://100.x.x.x。也可直接粘贴电脑端「连接」页面里的完整手机访问链接，令牌会加密保存在 iPhone 钥匙串中。",
                            en: "Use http://192.168.x.x:8721 on LAN, or Tailscale MagicDNS (http://mac.tailnet.ts.net:8721) / HTTPS (tailscale serve) instead of plaintext http://100.x.x.x. You can also paste the full phone link from Connect AI; your token is stored in the iOS Keychain."
                        ))
                    }
                }
            }
            .navigationTitle(L10n.pick(zh: "连接设置", en: "Connection Settings"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button(L10n.pick(zh: "取消", en: "Cancel")) {
                        dismiss()
                    }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button {
                        Task { await saveAndApply() }
                    } label: {
                        if isSaving {
                            ProgressView()
                                .controlSize(.small)
                        } else {
                            Text(selectedMode == .local
                                ? L10n.pick(zh: "使用本机", en: "Use iPhone")
                                : L10n.pick(zh: "保存并连接", en: "Save & Connect"))
                                .fontWeight(.semibold)
                        }
                    }
                    .disabled(isSaving || (selectedMode == .remote && serverURL.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty))
                }
            }
            .onAppear {
                selectedMode = appState.mode
                serverURL = appState.remoteBaseURL
                accessToken = appState.remoteToken
                testStatusMessage = nil
                testSucceeded = false
            }
        }
    }

    @ViewBuilder
    private func modeRow(mode: ConnectionMode, icon: String, title: String, subtitle: String) -> some View {
        Button {
            withAnimation(.easeInOut(duration: 0.18)) {
                selectedMode = mode
                testStatusMessage = nil
            }
        } label: {
            HStack(alignment: .top, spacing: 12) {
                Image(systemName: icon)
                    .font(.title3)
                    .foregroundStyle(selectedMode == mode ? Color.accentColor : .secondary)
                    .frame(width: 26)
                    .padding(.top, 2)

                VStack(alignment: .leading, spacing: 4) {
                    Text(title)
                        .font(.body.weight(.medium))
                        .foregroundStyle(.primary)
                    Text(subtitle)
                        .font(.caption)
                        .foregroundStyle(.secondary)
                        .fixedSize(horizontal: false, vertical: true)
                }

                Spacer(minLength: 8)

                Image(systemName: selectedMode == mode ? "checkmark.circle.fill" : "circle")
                    .font(.title3)
                    .foregroundStyle(selectedMode == mode ? Color.accentColor : Color.secondary.opacity(0.4))
                    .padding(.top, 2)
            }
            .contentShape(Rectangle())
            .padding(.vertical, 4)
        }
        .buttonStyle(.plain)
    }

    private func handlePossiblePhoneLinkPaste(_ value: String) {
        guard value.contains("token=") else { return }
        let parsed = AppState.parseServerInput(value)
        if let token = parsed.extractedToken, !token.isEmpty {
            serverURL = parsed.baseURL
            accessToken = token
        }
    }

    private func runConnectionTest() async {
        isTesting = true
        testStatusMessage = nil
        defer { isTesting = false }

        let result = await appState.testRemoteConnection(rawURL: serverURL, token: accessToken)
        switch result {
        case .success(let message):
            testSucceeded = true
            testStatusMessage = message
        case .failure(let error):
            testSucceeded = false
            testStatusMessage = error.localizedDescription
        }
    }

    private func saveAndApply() async {
        isSaving = true
        defer { isSaving = false }

        switch selectedMode {
        case .local:
            await appState.switchToLocal()
            dismiss()
        case .remote:
            do {
                try await appState.saveAndConnectRemote(rawURL: serverURL, token: accessToken)
                dismiss()
            } catch {
                testSucceeded = false
                testStatusMessage = error.localizedDescription
            }
        }
    }
}
