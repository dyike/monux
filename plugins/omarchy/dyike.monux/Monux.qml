import QtQuick
import Quickshell.Io
import qs.Commons
import qs.Ui

Panel {
  id: root
  moduleName: "dyike.monux"
  ipcTarget: "dyike.monux"

  property string currentName: ""
  property string currentInput: ""
  property string pendingName: ""
  property string lastError: ""
  property string statusWarning: ""
  property string statusOutput: ""
  property string statusError: ""
  property string switchOutput: ""
  property string switchError: ""
  property bool switchFailed: false
  property string settingsMessage: ""
  property string settingsError: ""
  property string configOutput: ""
  property string configError: ""
  property string serviceOutput: ""
  property string serviceError: ""
  property string serviceSuccessMessage: ""
  property string selectedTab: "inputs"
  property string focusSection: "tabs"
  property int selectedInputIndex: 0
  property int selectedSettingsAction: 0
  property bool draftShowLabel: false
  property bool cursorActive: false

  readonly property string executable: String(root.setting("executable", "monux")).trim()
  readonly property string configPath: String(root.setting("configPath", "")).trim()
  readonly property string primaryInput: String(root.setting("primaryInput", "linux")).trim()
  readonly property string secondaryInput: String(root.setting("secondaryInput", "mac")).trim()
  readonly property string tertiaryInput: String(root.setting("tertiaryInput", "windows")).trim()
  readonly property int refreshIntervalSec: boundedInteger(root.setting("refreshIntervalSec", 10), 10, 5, 3600)
  readonly property bool showLabel: root.setting("showLabel", false) === true
  readonly property bool busy: statusProcess.running || switchProcess.running || configProcess.running || serviceProcess.running
  readonly property var tabs: [
    { value: "inputs", label: "Inputs" },
    { value: "info", label: "Info" },
    { value: "settings", label: "Settings" }
  ]
  readonly property var inputOptions: buildInputOptions()
  readonly property string displayName: pendingName !== "" ? pendingName : (currentName !== "" ? currentName : (currentInput !== "" ? currentInput : "Choose input"))
  readonly property string stateLabel: lastError !== "" ? "SWITCH FAILED" : (busy ? "UPDATING" : "MONITOR INPUT")
  readonly property string tooltip: tooltipMessage()

  function boundedInteger(value, fallback, minimum, maximum) {
    var parsed = parseInt(String(value), 10)
    if (!isFinite(parsed)) parsed = fallback
    return Math.max(minimum, Math.min(maximum, parsed))
  }

  function titleCase(value) {
    var text = String(value || "")
    return text === "" ? "" : text.charAt(0).toUpperCase() + text.substring(1)
  }

  function inputIcon(value) {
    var name = String(value || "").toLowerCase()
    if (name.indexOf("mac") !== -1 || name.indexOf("apple") !== -1) return "\uf179"
    if (name.indexOf("linux") !== -1) return "\uf17c"
    if (name.indexOf("windows") !== -1 || name.indexOf("win") !== -1) return "\uf17a"
    return "\uf0ec"
  }

  function appendInputOption(options, name) {
    var value = String(name || "").trim()
    if (value === "") return
    for (var i = 0; i < options.length; i++)
      if (String(options[i].value) === value) return
    options.push({ value: value, label: titleCase(value), icon: inputIcon(value) })
  }

  function buildInputOptions() {
    var options = []
    appendInputOption(options, primaryInput)
    appendInputOption(options, secondaryInput)
    appendInputOption(options, tertiaryInput)
    return options
  }

  function optionIndex(name) {
    for (var i = 0; i < inputOptions.length; i++)
      if (String(inputOptions[i].value) === String(name)) return i
    return 0
  }

  function tabIndex(value) {
    for (var i = 0; i < tabs.length; i++)
      if (String(tabs[i].value) === String(value)) return i
    return 0
  }

  function monuxCommand(argumentsList) {
    var command = [executable]
    if (configPath !== "") command.push("--config", configPath)
    for (var i = 0; i < argumentsList.length; i++) command.push(argumentsList[i])
    return command
  }

  function compactError(value, fallback) {
    var message = String(value || "").replace(/\s+/g, " ").trim()
    if (message === "") message = fallback
    if (message.length > 240) message = message.substring(0, 237) + "…"
    return message
  }

  function applyStatus(raw) {
    var value = String(raw || "").trim()
    var named = value.match(/^(.*)\s+\((0x[0-9a-fA-F]+)\)$/)
    if (named) {
      currentName = String(named[1]).trim()
      currentInput = String(named[2]).toLowerCase()
      selectedInputIndex = optionIndex(currentName)
    } else if (/^0x[0-9a-fA-F]+$/.test(value)) {
      currentName = ""
      currentInput = value.toLowerCase()
    } else {
      statusWarning = "Unexpected monux status: " + compactError(value, "empty output")
      return
    }
    pendingName = ""
    lastError = ""
    statusWarning = ""
    switchFailed = false
  }

  function refresh() {
    if (busy || executable === "") return
    switchFailed = false
    lastError = ""
    statusWarning = ""
    statusOutput = ""
    statusError = ""
    statusProcess.command = monuxCommand(["status"])
    statusProcess.running = true
    statusTimeout.restart()
  }

  function switchTo(name) {
    var target = String(name || "").trim()
    if (busy || executable === "") return
    if (target === "") {
      lastError = "Configure at least one Monux input name"
      return
    }
    pendingName = target
    lastError = ""
    switchFailed = false
    switchOutput = ""
    switchError = ""
    switchProcess.command = monuxCommand(["switch", target])
    switchProcess.running = true
    switchTimeout.restart()
  }

  function syncSettingsFields() {
    primaryInputField.text = primaryInput
    secondaryInputField.text = secondaryInput
    tertiaryInputField.text = tertiaryInput
    refreshField.value = refreshIntervalSec
    draftShowLabel = showLabel
  }

  function persistSettings(values) {
    var entry = { id: root.moduleName }
    for (var existing in root.settings)
      if (existing !== "id") entry[existing] = root.settings[existing]
    for (var key in values) entry[key] = values[key]

    root.settings = entry
    if (root.bar && root.bar.shell && typeof root.bar.shell.updateEntryInline === "function") {
      root.bar.shell.updateEntryInline(root.moduleName, entry)
      return true
    }
    return false
  }

  function savePluginSettings() {
    var primary = primaryInputField.text.trim()
    var secondary = secondaryInputField.text.trim()
    var tertiary = tertiaryInputField.text.trim()
    settingsError = ""
    settingsMessage = ""
    if (primary === "" || secondary === "" || tertiary === "") {
      settingsError = "All three input names are required"
      return
    }
    if (primary === secondary || primary === tertiary || secondary === tertiary) {
      settingsError = "Input names must be different"
      return
    }
    var persisted = persistSettings({
      primaryInput: primary,
      secondaryInput: secondary,
      tertiaryInput: tertiary,
      refreshIntervalSec: refreshField.value,
      showLabel: draftShowLabel
    })
    settingsMessage = persisted ? "Plugin settings saved" : "Settings applied for this session"
    selectedInputIndex = optionIndex(currentName)
    keyCatcher.forceActiveFocus()
  }

  function refreshMonitorConfig() {
    if (busy || executable === "") return
    settingsError = ""
    settingsMessage = ""
    configOutput = ""
    configError = ""
    configProcess.command = monuxCommand(["init"])
    configProcess.running = true
  }

  function restartService(successMessage) {
    if (serviceProcess.running) return
    settingsError = ""
    serviceOutput = ""
    serviceError = ""
    serviceSuccessMessage = String(successMessage || "HTTP service restarted")
    serviceProcess.command = ["systemctl", "--user", "restart", "monux.service"]
    serviceProcess.running = true
  }

  function tooltipMessage() {
    if (lastError !== "") return "Monux: " + lastError + "\nRight click to retry"
    if (statusWarning !== "") return "Current input reading unavailable\nSwitching remains available"
    var selected = currentName !== "" ? currentName : (currentInput !== "" ? currentInput : "checking…")
    return "Monitor input: " + selected + "\nLeft click: open controls\nRight click: refresh"
  }

  function moveCursor(dx, dy) {
    cursorActive = true
    if (dy !== 0) {
      if (dy > 0 && focusSection === "tabs") {
        if (selectedTab === "inputs") focusSection = "inputChoices"
        else if (selectedTab === "info") focusSection = "infoAction"
        else focusSection = "settingsAction"
      }
      else if (dy < 0 && focusSection !== "tabs") focusSection = "tabs"
      return
    }
    if (dx === 0) return
    if (focusSection === "tabs") {
      var nextTab = Math.max(0, Math.min(tabs.length - 1, tabIndex(selectedTab) + dx))
      selectedTab = tabs[nextTab].value
      if (selectedTab === "settings") Qt.callLater(syncSettingsFields)
      return
    }
    if (focusSection === "inputChoices" && inputOptions.length > 0) {
      selectedInputIndex = Math.max(0, Math.min(inputOptions.length - 1, selectedInputIndex + dx))
    } else if (focusSection === "settingsAction") {
      selectedSettingsAction = Math.max(0, Math.min(2, selectedSettingsAction + dx))
    }
  }

  function activateCursor() {
    if (!cursorActive) return
    if (focusSection === "inputChoices" && selectedInputIndex >= 0 && selectedInputIndex < inputOptions.length)
      switchTo(inputOptions[selectedInputIndex].value)
    else if (focusSection === "infoAction") refresh()
    else if (focusSection === "settingsAction") {
      if (selectedSettingsAction === 0) savePluginSettings()
      else if (selectedSettingsAction === 1) refreshMonitorConfig()
      else restartService("HTTP service restarted")
    }
  }

  implicitWidth: button.implicitWidth
  implicitHeight: button.implicitHeight

  onOpenedChanged: if (opened) {
    cursorActive = false
    focusSection = "tabs"
    selectedInputIndex = optionIndex(currentName)
    refresh()
  }

  Process {
    id: statusProcess
    running: false
    command: []
    stdout: StdioCollector {
      id: statusStdout
      waitForEnd: true
      onStreamFinished: root.statusOutput = text
    }
    stderr: StdioCollector {
      id: statusStderr
      waitForEnd: true
      onStreamFinished: root.statusError = text
    }
    onExited: function(exitCode) {
      statusTimeout.stop()
      if (exitCode === 0) root.applyStatus(statusStdout.text || root.statusOutput)
      else {
        root.switchFailed = false
        root.statusWarning = root.compactError(statusStderr.text || root.statusError || statusStdout.text || root.statusOutput, "monux status failed")
      }
    }
  }

  Process {
    id: configProcess
    running: false
    command: []
    stdout: StdioCollector {
      id: configStdout
      waitForEnd: true
      onStreamFinished: root.configOutput = text
    }
    stderr: StdioCollector {
      id: configStderr
      waitForEnd: true
      onStreamFinished: root.configError = text
    }
    onExited: function(exitCode) {
      if (exitCode === 0) root.restartService("Monitor configuration refreshed and HTTP service restarted")
      else root.settingsError = root.compactError(configStderr.text || root.configError || configStdout.text || root.configOutput, "monux init failed")
    }
  }

  Process {
    id: serviceProcess
    running: false
    command: []
    stdout: StdioCollector {
      id: serviceStdout
      waitForEnd: true
      onStreamFinished: root.serviceOutput = text
    }
    stderr: StdioCollector {
      id: serviceStderr
      waitForEnd: true
      onStreamFinished: root.serviceError = text
    }
    onExited: function(exitCode) {
      if (exitCode === 0) {
        root.settingsError = ""
        root.settingsMessage = root.serviceSuccessMessage
        root.refresh()
      } else {
        root.settingsError = root.compactError(serviceStderr.text || root.serviceError || serviceStdout.text || root.serviceOutput, "could not restart monux.service")
      }
    }
  }

  Process {
    id: switchProcess
    running: false
    command: []
    stdout: StdioCollector {
      id: switchStdout
      waitForEnd: true
      onStreamFinished: root.switchOutput = text
    }
    stderr: StdioCollector {
      id: switchStderr
      waitForEnd: true
      onStreamFinished: root.switchError = text
    }
    onExited: function(exitCode) {
      switchTimeout.stop()
      if (exitCode === 0) {
        root.currentName = root.pendingName
        root.currentInput = ""
        root.selectedInputIndex = root.optionIndex(root.currentName)
        root.pendingName = ""
        root.lastError = ""
        root.statusWarning = ""
        root.switchFailed = false
      } else {
        root.pendingName = ""
        root.switchFailed = true
        root.lastError = root.compactError(switchStderr.text || root.switchError || switchStdout.text || root.switchOutput, "monux switch failed")
      }
    }
  }

  Timer {
    interval: root.refreshIntervalSec * 1000
    running: root.opened && root.statusWarning === ""
    repeat: true
    triggeredOnStart: false
    onTriggered: root.refresh()
  }

  Timer {
    id: statusTimeout
    interval: 8000
    repeat: false
    onTriggered: {
      if (statusProcess.running) statusProcess.running = false
      root.statusWarning = "monux status timed out"
    }
  }

  Timer {
    id: switchTimeout
    interval: 8000
    repeat: false
    onTriggered: {
      if (switchProcess.running) switchProcess.running = false
      root.pendingName = ""
      root.switchFailed = true
      root.lastError = "monux switch timed out"
    }
  }

  WidgetButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: "\uf0ec" + (root.showLabel && !vertical && root.currentName !== "" ? "  " + root.currentName : "")
    fontSize: root.showLabel && !vertical ? Style.font.body : Style.bar.iconFont
    fixedWidth: root.showLabel && !vertical ? -1 : Style.bar.iconSlot
    fixedHeight: vertical ? Style.bar.iconSlot : -1
    active: root.lastError !== ""
    dimmed: root.busy
    tooltipText: root.tooltip
    onPressed: function(buttonCode) {
      if (buttonCode === Qt.RightButton) root.refresh()
      else root.toggle()
    }
  }

  KeyboardPanel {
    id: panel
    anchorItem: button
    owner: root
    bar: root.bar
    open: root.opened
    focusTarget: keyCatcher
    contentWidth: panel.fittedContentWidth(Style.space(360))
    contentHeight: panel.fittedContentHeight(panelColumn.implicitHeight, Style.space(520))

    PanelKeyCatcher {
      id: keyCatcher
      anchors.fill: parent
      onMoveRequested: function(dx, dy) { root.moveCursor(dx, dy) }
      onActivateRequested: root.activateCursor()
      onCloseRequested: root.close()
      onTabRequested: function(direction) { root.switchPanel(direction) }
      onTextKey: function(text) { if (text === "r" || text === "R") root.refresh() }

      Column {
        id: panelColumn
        width: parent.width
        spacing: Style.space(14)

        PanelHero {
          width: parent.width
          iconComponent: Component {
            Text {
              text: "\uf0ec"
              color: root.bar.foreground
              font.family: root.bar.fontFamily
              font.pixelSize: Style.font.display
            }
          }
          title: "Monux"
          meta: root.stateLabel
          detail: root.displayName
          foreground: root.bar.foreground
          fontFamily: root.bar.fontFamily
        }

        PanelSeparator { foreground: root.bar.foreground }

        ButtonGroup {
          id: tabs
          anchors.horizontalCenter: parent.horizontalCenter
          options: root.tabs
          value: root.selectedTab
          cursorIndex: root.cursorActive && root.focusSection === "tabs" ? root.tabIndex(root.selectedTab) : -1
          foreground: root.bar.foreground
          fontFamily: root.bar.fontFamily
          onChanged: function(value) {
            root.selectedTab = value
            root.focusSection = "tabs"
            root.cursorActive = true
            if (value === "settings") Qt.callLater(root.syncSettingsFields)
          }
        }

        Column {
          width: parent.width
          spacing: Style.space(12)
          visible: root.selectedTab === "inputs"

          PanelSectionHeader {
            text: "CURRENT INPUT"
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
          }

          InfoRow {
            label: "Name"
            value: root.currentName !== "" ? root.currentName : "Not available"
          }

          InfoRow {
            label: "VCP value"
            value: root.currentInput !== "" ? root.currentInput : "—"
          }

          Text {
            width: parent.width
            visible: root.statusWarning !== ""
            text: "Current input cannot be read on this DDC link. You can still choose an input below."
            color: Qt.darker(root.bar.foreground, 1.4)
            font.family: root.bar.fontFamily
            font.pixelSize: Style.font.caption
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.WordWrap
          }

          PanelSeparator { foreground: root.bar.foreground }

          PanelSectionHeader {
            text: "SWITCH INPUT"
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
          }

          ButtonGroup {
            id: inputChoices
            anchors.horizontalCenter: parent.horizontalCenter
            options: root.inputOptions
            value: root.pendingName !== "" ? root.pendingName : root.currentName
            cursorIndex: root.cursorActive && root.focusSection === "inputChoices" ? root.selectedInputIndex : -1
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
            onChanged: function(value) { root.switchTo(value) }
            onHovered: function(index, isHovered) {
              if (!isHovered) return
              root.cursorActive = true
              root.focusSection = "inputChoices"
              root.selectedInputIndex = index
            }
          }

          Text {
            width: parent.width
            text: root.busy ? "Applying monitor input…" : "Choose a configured computer input."
            color: Qt.darker(root.bar.foreground, 1.4)
            font.family: root.bar.fontFamily
            font.pixelSize: Style.font.caption
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.WordWrap
          }

        }

        Column {
          width: parent.width
          spacing: Style.space(10)
          visible: root.selectedTab === "settings"

          PanelSectionHeader {
            text: "PLUGIN"
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
          }

          Text {
            width: parent.width
            text: "Input button names"
            color: Qt.darker(root.bar.foreground, 1.4)
            font.family: root.bar.fontFamily
            font.pixelSize: Style.font.caption
          }

          Row {
            width: parent.width
            spacing: Style.space(6)

            TextField {
              id: primaryInputField
              width: (parent.width - parent.spacing * 2) / 3
              placeholderText: "linux"
              foreground: root.bar.foreground
              font.family: root.bar.fontFamily
            }

            TextField {
              id: secondaryInputField
              width: (parent.width - parent.spacing * 2) / 3
              placeholderText: "mac"
              foreground: root.bar.foreground
              font.family: root.bar.fontFamily
            }

            TextField {
              id: tertiaryInputField
              width: (parent.width - parent.spacing * 2) / 3
              placeholderText: "windows"
              foreground: root.bar.foreground
              font.family: root.bar.fontFamily
            }
          }

          Row {
            width: parent.width
            spacing: Style.space(10)

            NumberField {
              id: refreshField
              width: parent.width * 0.36
              label: "Refresh seconds"
              from: 5
              to: 3600
              stepSize: 5
              value: 10
              foreground: root.bar.foreground
              fontFamily: root.bar.fontFamily
            }

            Toggle {
              width: parent.width - refreshField.width - parent.spacing
              label: "Show name in bar"
              checked: root.draftShowLabel
              foreground: root.bar.foreground
              fontFamily: root.bar.fontFamily
              onClicked: root.draftShowLabel = !root.draftShowLabel
            }
          }

          Row {
            width: parent.width
            spacing: Style.space(6)

            Button {
              width: (parent.width - parent.spacing * 2) / 3
              text: "Save"
              iconText: "\uf0c7"
              bordered: true
              hasCursor: root.cursorActive && root.focusSection === "settingsAction" && root.selectedSettingsAction === 0
              foreground: root.bar.foreground
              fontFamily: root.bar.fontFamily
              onClicked: root.savePluginSettings()
              onHovered: function(isHovered) {
                if (!isHovered) return
                root.cursorActive = true
                root.focusSection = "settingsAction"
                root.selectedSettingsAction = 0
              }
            }

            Button {
              width: (parent.width - parent.spacing * 2) / 3
              text: "Detect"
              iconText: "\uf002"
              bordered: true
              enabled: !root.busy
              hasCursor: root.cursorActive && root.focusSection === "settingsAction" && root.selectedSettingsAction === 1
              foreground: root.bar.foreground
              fontFamily: root.bar.fontFamily
              onClicked: root.refreshMonitorConfig()
              onHovered: function(isHovered) {
                if (!isHovered) return
                root.cursorActive = true
                root.focusSection = "settingsAction"
                root.selectedSettingsAction = 1
              }
            }

            Button {
              width: (parent.width - parent.spacing * 2) / 3
              text: "Restart"
              iconText: "\uf021"
              bordered: true
              enabled: !root.busy
              hasCursor: root.cursorActive && root.focusSection === "settingsAction" && root.selectedSettingsAction === 2
              foreground: root.bar.foreground
              fontFamily: root.bar.fontFamily
              onClicked: root.restartService("HTTP service restarted")
              onHovered: function(isHovered) {
                if (!isHovered) return
                root.cursorActive = true
                root.focusSection = "settingsAction"
                root.selectedSettingsAction = 2
              }
            }
          }

          Text {
            width: parent.width
            visible: root.settingsMessage !== "" || root.settingsError !== ""
            text: root.settingsError !== "" ? root.settingsError : root.settingsMessage
            color: root.settingsError !== "" ? root.bar.urgent : Qt.darker(root.bar.foreground, 1.25)
            font.family: root.bar.fontFamily
            font.pixelSize: Style.font.caption
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.WordWrap
          }

          Text {
            width: parent.width
            text: "Detect runs monux init and restarts the managed HTTP service. Peer URLs and connector mappings remain owned by the Monux CLI configuration."
            color: Qt.darker(root.bar.foreground, 1.55)
            font.family: root.bar.fontFamily
            font.pixelSize: Style.font.caption
            horizontalAlignment: Text.AlignHCenter
            wrapMode: Text.WordWrap
          }
        }

        Column {
          width: parent.width
          spacing: Style.space(10)
          visible: root.selectedTab === "info"

          PanelSectionHeader {
            text: "MONITOR CONTROL"
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
          }

          InfoRow { label: "Transport"; value: "Local DDC + peer fallback" }
          InfoRow { label: "Input reading"; value: root.statusWarning === "" ? "Available" : "Unavailable" }
          InfoRow { label: "Current value"; value: root.currentInput !== "" ? root.currentInput : "Unknown" }
          InfoRow { label: "Refresh"; value: root.refreshIntervalSec + " seconds" }
          InfoRow { label: "CLI"; value: root.executable }
          InfoRow { label: "Config"; value: root.configPath !== "" ? root.configPath : "Platform default" }

          Button {
            width: parent.width
            text: root.busy ? "Refreshing…" : "Refresh now"
            iconText: "\uf021"
            bordered: true
            hasCursor: root.cursorActive && root.focusSection === "infoAction"
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
            onClicked: root.refresh()
            onHovered: function(isHovered) {
              if (!isHovered) return
              root.cursorActive = true
              root.focusSection = "infoAction"
            }
          }
        }

        ErrorCard { visible: root.lastError !== "" }

      }
    }
  }

  component InfoRow: Item {
    id: infoRow

    required property string label
    required property string value

    width: parent ? parent.width : implicitWidth
    implicitHeight: Math.max(labelText.implicitHeight, valueText.implicitHeight)

    Text {
      id: labelText
      anchors.left: parent.left
      anchors.verticalCenter: parent.verticalCenter
      width: parent.width * 0.34
      text: infoRow.label
      color: Qt.darker(root.bar.foreground, 1.4)
      font.family: root.bar.fontFamily
      font.pixelSize: Style.font.body
      elide: Text.ElideRight
    }

    Text {
      id: valueText
      anchors.left: labelText.right
      anchors.right: parent.right
      anchors.verticalCenter: parent.verticalCenter
      text: infoRow.value
      color: root.bar.foreground
      font.family: root.bar.fontFamily
      font.pixelSize: Style.font.body
      horizontalAlignment: Text.AlignRight
      elide: Text.ElideMiddle
    }
  }

  component ErrorCard: Rectangle {
    width: parent ? parent.width : implicitWidth
    implicitHeight: errorText.implicitHeight + Style.space(16)
    radius: Style.cornerRadius
    color: Style.hoverFillFor(root.bar.urgent, root.bar.urgent)
    border.width: 1
    border.color: root.bar.urgent

    Text {
      id: errorText
      anchors.fill: parent
      anchors.margins: Style.space(8)
      text: "Input switch failed.\n" + root.lastError
      color: root.bar.urgent
      font.family: root.bar.fontFamily
      font.pixelSize: Style.font.caption
      wrapMode: Text.WordWrap
      verticalAlignment: Text.AlignVCenter
    }
  }
}
