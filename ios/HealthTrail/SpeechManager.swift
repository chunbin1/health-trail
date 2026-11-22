import AVFoundation
import Foundation
import Speech

struct SpeechBridgeEvent: Encodable {
    let type: String
    let text: String?
    let error: String?

    init(type: String, text: String? = nil, error: String? = nil) {
        self.type = type
        self.text = text
        self.error = error
    }
}

@MainActor
final class SpeechManager {
    var onEvent: ((SpeechBridgeEvent) -> Void)?

    private let audioEngine = AVAudioEngine()
    private var recognitionRequest: SFSpeechAudioBufferRecognitionRequest?
    private var recognitionTask: SFSpeechRecognitionTask?
    private var isListening = false
    private var lastTranscript = ""
    private var hasTapInstalled = false
    private var sessionToken = UUID()

    func start(lang: String?) {
        stop(emitStopped: false)
        let currentToken = UUID()
        sessionToken = currentToken

        Task { @MainActor [weak self] in
            guard let self, self.sessionToken == currentToken else { return }

            let speechStatus = await Self.requestSpeechAuthorization()
            guard self.sessionToken == currentToken else { return }
            guard speechStatus == .authorized else {
                self.emit(.init(
                    type: "error",
                    error: L10n.pick(
                        zh: "请在系统「设置 → 健康足迹」中开启语音识别权限。",
                        en: "Please enable Speech Recognition permission in Settings → Health Trail."
                    )
                ))
                return
            }

            let micGranted = await Self.requestMicrophonePermission()
            guard self.sessionToken == currentToken else { return }
            guard micGranted else {
                self.emit(.init(
                    type: "error",
                    error: L10n.pick(
                        zh: "请在系统「设置 → 健康足迹」中开启麦克风权限。",
                        en: "Please enable Microphone permission in Settings → Health Trail."
                    )
                ))
                return
            }

            self.beginRecognition(lang: lang, token: currentToken)
        }
    }

    func stop(emitStopped: Bool = true) {
        let wasActive = isListening || recognitionTask != nil || recognitionRequest != nil
        sessionToken = UUID()
        isListening = false

        let finalTranscript = lastTranscript
        cleanupAudioAndTask()
        lastTranscript = ""

        if wasActive && emitStopped {
            if !finalTranscript.isEmpty {
                emit(.init(type: "final", text: finalTranscript))
            }
            emit(.init(type: "stopped"))
        }
    }

    private func beginRecognition(lang: String?, token: UUID) {
        let localeID = (lang?.lowercased() == "en") ? "en-US" : "zh-CN"
        guard let recognizer = SFSpeechRecognizer(locale: Locale(identifier: localeID)), recognizer.isAvailable else {
            emit(.init(
                type: "error",
                error: L10n.pick(
                    zh: "当前设备暂不支持语音识别服务。",
                    en: "Speech recognition is currently unavailable on this device."
                )
            ))
            return
        }

        let audioSession = AVAudioSession.sharedInstance()
        do {
            try audioSession.setCategory(.record, mode: .measurement, options: .duckOthers)
            try audioSession.setActive(true, options: .notifyOthersOnDeactivation)
        } catch {
            emit(.init(
                type: "error",
                error: L10n.pick(
                    zh: "无法启动麦克风音频会话：\(error.localizedDescription)",
                    en: "Unable to start microphone audio session: \(error.localizedDescription)"
                )
            ))
            return
        }

        let inputNode = audioEngine.inputNode
        let recordingFormat = inputNode.outputFormat(forBus: 0)
        // Guard against invalid hardware format (e.g., simulator without microphone input)
        // to avoid an uncatchable ObjC NSException from installTap.
        guard recordingFormat.channelCount > 0, recordingFormat.sampleRate > 0 else {
            deactivateAudioSession()
            emit(.init(
                type: "error",
                error: L10n.pick(
                    zh: "未检测到可用的麦克风输入设备。",
                    en: "No microphone input device is available."
                )
            ))
            return
        }

        let request = SFSpeechAudioBufferRecognitionRequest()
        request.shouldReportPartialResults = true
        if recognizer.supportsOnDeviceRecognition {
            request.requiresOnDeviceRecognition = true
        }
        if #available(iOS 16.0, *) {
            request.addsPunctuation = true
        }
        recognitionRequest = request
        lastTranscript = ""

        inputNode.removeTap(onBus: 0)
        inputNode.installTap(onBus: 0, bufferSize: 1024, format: recordingFormat) { [weak request] buffer, _ in
            request?.append(buffer)
        }
        hasTapInstalled = true

        audioEngine.prepare()
        do {
            try audioEngine.start()
        } catch {
            cleanupAudioAndTask()
            emit(.init(
                type: "error",
                error: L10n.pick(
                    zh: "无法启动录音引擎：\(error.localizedDescription)",
                    en: "Unable to start audio recording engine: \(error.localizedDescription)"
                )
            ))
            return
        }

        isListening = true
        emit(.init(type: "started"))

        recognitionTask = recognizer.recognitionTask(with: request) { [weak self] result, error in
            Task { @MainActor [weak self] in
                guard let self, self.sessionToken == token, self.isListening else { return }

                if let result {
                    let transcript = result.bestTranscription.formattedString
                    self.lastTranscript = transcript
                    if result.isFinal {
                        self.isListening = false
                        self.cleanupAudioAndTask()
                        self.lastTranscript = ""
                        self.emit(.init(type: "final", text: transcript))
                        self.emit(.init(type: "stopped"))
                        return
                    } else {
                        self.emit(.init(type: "partial", text: transcript))
                    }
                }

                if let error {
                    self.isListening = false
                    let partial = self.lastTranscript
                    self.cleanupAudioAndTask()
                    self.lastTranscript = ""
                    if !partial.isEmpty {
                        self.emit(.init(type: "final", text: partial))
                        self.emit(.init(type: "stopped"))
                    } else {
                        self.emit(.init(type: "error", error: error.localizedDescription))
                    }
                }
            }
        }
    }

    private func cleanupAudioAndTask() {
        if audioEngine.isRunning {
            audioEngine.stop()
        }
        if hasTapInstalled {
            audioEngine.inputNode.removeTap(onBus: 0)
            hasTapInstalled = false
        }
        recognitionRequest?.endAudio()
        recognitionRequest = nil
        recognitionTask?.cancel()
        recognitionTask = nil
        deactivateAudioSession()
    }

    private func deactivateAudioSession() {
        try? AVAudioSession.sharedInstance().setActive(false, options: .notifyOthersOnDeactivation)
    }

    private func emit(_ event: SpeechBridgeEvent) {
        onEvent?(event)
    }

    private static func requestSpeechAuthorization() async -> SFSpeechRecognizerAuthorizationStatus {
        let current = SFSpeechRecognizer.authorizationStatus()
        if current != .notDetermined {
            return current
        }
        return await withCheckedContinuation { continuation in
            SFSpeechRecognizer.requestAuthorization { status in
                continuation.resume(returning: status)
            }
        }
    }

    private static func requestMicrophonePermission() async -> Bool {
        let session = AVAudioSession.sharedInstance()
        switch session.recordPermission {
        case .granted:
            return true
        case .denied:
            return false
        case .undetermined:
            return await withCheckedContinuation { continuation in
                session.requestRecordPermission { granted in
                    continuation.resume(returning: granted)
                }
            }
        @unknown default:
            return false
        }
    }
}
