import SwiftUI

@main
struct HealthTrailApp: App {
    @StateObject private var appState = AppState()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView(appState: appState)
                .task {
                    await appState.startIfNeeded()
                }
                .onChange(of: scenePhase) { newPhase in
                    if newPhase == .active {
                        Task {
                            await appState.handleBecomeActive()
                        }
                    } else if newPhase == .background {
                        appState.speechManager.stop()
                        appState.armBreakReminder()
                    }
                }
        }
    }
}
