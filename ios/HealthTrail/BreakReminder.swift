import Foundation
import UserNotifications

/// Schedules the water reminder as a real local notification. The break timer
/// itself only ticks while this process is running; iOS suspends that as soon
/// as the app leaves the foreground, so the banner has to be armed ahead of time.
final class BreakReminder: NSObject, UNUserNotificationCenterDelegate {
    static let shared = BreakReminder()

    private let requestID = "healthtrail.break.water"
    private let lock = NSLock()
    private var baseURL: URL?
    private var poll: Task<Void, Never>?
    private var armedUntil: Date?
    private var armedSound = false

    private override init() {
        super.init()
    }

    func bind(baseURL: URL) {
        setBaseURL(baseURL)
        UNUserNotificationCenter.current().delegate = self
        if poll == nil {
            poll = Task { [weak self] in
                while !Task.isCancelled {
                    await self?.sync()
                    try? await Task.sleep(nanoseconds: 5_000_000_000)
                }
            }
        }
        Task { await sync() }
    }

    func unbind() {
        clearSchedule()
        poll?.cancel()
        poll = nil
        UNUserNotificationCenter.current().removePendingNotificationRequests(withIdentifiers: [requestID])
    }

    func syncNow() {
        Task { await sync() }
    }

    func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification,
        withCompletionHandler completionHandler: @escaping (UNNotificationPresentationOptions) -> Void
    ) {
        var options: UNNotificationPresentationOptions = [.banner, .list]
        if notification.request.content.sound != nil {
            options.insert(.sound)
        }
        completionHandler(options)
    }

    private struct Snapshot: Decodable {
        let state: String
        let remainingSec: Int
        let config: Config

        struct Config: Decodable {
            let soundEnabled: Bool

            enum CodingKeys: String, CodingKey {
                case soundEnabled = "sound_enabled"
            }
        }

        enum CodingKeys: String, CodingKey {
            case state
            case remainingSec = "remaining_sec"
            case config
        }
    }

    private func setBaseURL(_ url: URL) {
        lock.lock()
        baseURL = url
        lock.unlock()
    }

    private func clearSchedule() {
        lock.lock()
        baseURL = nil
        armedUntil = nil
        lock.unlock()
    }

    private func currentBase() -> URL? {
        lock.lock()
        defer { lock.unlock() }
        return baseURL
    }

    private func rememberedFire() -> (date: Date, sound: Bool)? {
        lock.lock()
        defer { lock.unlock() }
        guard let armedUntil else { return nil }
        return (armedUntil, armedSound)
    }

    private func remember(fire: Date, sound: Bool) {
        lock.lock()
        armedUntil = fire
        armedSound = sound
        lock.unlock()
    }

    private func forgetFire() {
        lock.lock()
        armedUntil = nil
        lock.unlock()
    }

    private func sync() async {
        guard let base = currentBase() else { return }

        var req = URLRequest(url: base.appendingPathComponent("api/break-timer"))
        req.timeoutInterval = 2
        req.cachePolicy = .reloadIgnoringLocalAndRemoteCacheData
        guard
            let (data, response) = try? await URLSession.shared.data(for: req),
            let http = response as? HTTPURLResponse,
            (200..<300).contains(http.statusCode),
            let snap = try? JSONDecoder().decode(Snapshot.self, from: data)
        else { return }

        if snap.state == "working", snap.remainingSec > 0 {
            await arm(after: snap.remainingSec, sound: snap.config.soundEnabled)
        } else {
            disarm()
        }
    }

    private func arm(after seconds: Int, sound: Bool) async {
        let interval = TimeInterval(max(1, seconds))
        let fire = Date().addingTimeInterval(interval)
        if let previous = rememberedFire(), previous.sound == sound, abs(previous.date.timeIntervalSince(fire)) < 3 {
            return
        }

        let center = UNUserNotificationCenter.current()
        let settings = await center.notificationSettings()
        switch settings.authorizationStatus {
        case .notDetermined:
            let granted = (try? await center.requestAuthorization(options: [.alert, .sound])) ?? false
            if !granted { return }
        case .denied:
            return
        default:
            break
        }

        let content = UNMutableNotificationContent()
        content.title = L10n.pick(zh: "该喝水了", en: "Time for water")
        content.body = L10n.pick(
            zh: "起身活动一下，喝口水，放松眼睛和肩颈。",
            en: "Stand up, have a drink, and rest your eyes and neck."
        )
        if sound {
            content.sound = .default
        }
        let trigger = UNTimeIntervalNotificationTrigger(timeInterval: interval, repeats: false)
        let request = UNNotificationRequest(identifier: requestID, content: content, trigger: trigger)
        do {
            try await center.add(request)
        } catch {
            return
        }
        remember(fire: fire, sound: sound)
    }

    private func disarm() {
        forgetFire()
        UNUserNotificationCenter.current().removePendingNotificationRequests(withIdentifiers: [requestID])
    }
}
