import Foundation

#if canImport(HealthTrailCore)
import HealthTrailCore
#endif

/// Isolates calls to the gomobile-generated HealthTrailCore.xcframework.
enum LocalCoreBridge {
    enum BridgeError: LocalizedError {
        case frameworkUnavailable
        case invalidURL(String)

        var errorDescription: String? {
            switch self {
            case .frameworkUnavailable:
                return L10n.pick(
                    zh: "本地核心组件未加载，请重新编译 HealthTrailCore.xcframework。",
                    en: "Local core framework is unavailable. Please build HealthTrailCore.xcframework."
                )
            case .invalidURL(let raw):
                return L10n.pick(
                    zh: "本地服务返回了无效地址：\(raw)",
                    en: "Local server returned an invalid address: \(raw)"
                )
            }
        }
    }

    static func start(dataDir: String) throws -> URL {
        // Ensure local health database, attachments, and AI keys are excluded from iCloud/system backups.
        var dirURL = URL(fileURLWithPath: dataDir, isDirectory: true)
        var values = URLResourceValues()
        values.isExcludedFromBackup = true
        try dirURL.setResourceValues(values)

        #if canImport(HealthTrailCore)
        var err: NSError?
        let rawURL = MobileStart(dataDir, &err)
        if let err {
            throw err
        }
        guard let url = URL(string: rawURL), !rawURL.isEmpty else {
            throw BridgeError.invalidURL(rawURL)
        }
        return url
        #else
        throw BridgeError.frameworkUnavailable
        #endif
    }

    static func stop() {
        #if canImport(HealthTrailCore)
        var err: NSError?
        MobileStop(&err)
        #endif
    }

    static func isRunning() -> Bool {
        #if canImport(HealthTrailCore)
        return MobileIsRunning()
        #else
        return false
        #endif
    }
}
