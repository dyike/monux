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
  property string selectedTab: "inputs"
  property string focusSection: "tabs"
  property int selectedInputIndex: 0
  property bool cursorActive: false

  readonly property string executable: String(root.setting("executable", "monux")).trim()
  readonly property string configPath: String(root.setting("configPath", "")).trim()
  readonly property string primaryInput: String(root.setting("primaryInput", "linux")).trim()
  readonly property string secondaryInput: String(root.setting("secondaryInput", "mac")).trim()
  readonly property string tertiaryInput: String(root.setting("tertiaryInput", "windows")).trim()
  readonly property int refreshIntervalSec: boundedInteger(root.setting("refreshIntervalSec", 10), 10, 5, 3600)
  readonly property bool busy: statusProcess.running || switchProcess.running
  readonly property var tabs: [
    { value: "inputs", label: "Inputs" },
    { value: "info", label: "Info" }
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

  function tooltipMessage() {
    if (lastError !== "") return "Monux: " + lastError + "\nRight click to retry"
    if (statusWarning !== "") return "Current input reading unavailable\nSwitching remains available"
    var selected = currentName !== "" ? currentName : (currentInput !== "" ? currentInput : "checking…")
    return "Monitor input: " + selected + "\nLeft click: open controls\nRight click: refresh"
  }

  function moveCursor(dx, dy) {
    cursorActive = true
    if (dy !== 0) {
      if (dy > 0 && focusSection === "tabs") focusSection = selectedTab === "inputs" ? "inputChoices" : "infoAction"
      else if (dy < 0 && focusSection !== "tabs") focusSection = "tabs"
      return
    }
    if (dx === 0) return
    if (focusSection === "tabs") {
      selectedTab = selectedTab === "inputs" ? "info" : "inputs"
      return
    }
    if (focusSection === "inputChoices" && inputOptions.length > 0) {
      selectedInputIndex = Math.max(0, Math.min(inputOptions.length - 1, selectedInputIndex + dx))
    }
  }

  function activateCursor() {
    if (!cursorActive) return
    if (focusSection === "inputChoices" && selectedInputIndex >= 0 && selectedInputIndex < inputOptions.length)
      switchTo(inputOptions[selectedInputIndex].value)
    else if (focusSection === "infoAction") refresh()
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

  BarIconButton {
    id: button
    anchors.fill: parent
    bar: root.bar
    text: "\uf0ec"
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
          cursorIndex: root.cursorActive && root.focusSection === "tabs" ? (root.selectedTab === "inputs" ? 0 : 1) : -1
          foreground: root.bar.foreground
          fontFamily: root.bar.fontFamily
          onChanged: function(value) {
            root.selectedTab = value
            root.focusSection = "tabs"
            root.cursorActive = true
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
          visible: root.selectedTab === "info"

          PanelSectionHeader {
            text: "MONITOR CONTROL"
            foreground: root.bar.foreground
            fontFamily: root.bar.fontFamily
          }

          InfoRow { label: "Transport"; value: "Local DDC/CI" }
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

        Text {
          width: parent.width
          text: "h/l navigate · j/k move · enter select · r refresh · esc close"
          color: Qt.darker(root.bar.foreground, 1.55)
          font.family: root.bar.fontFamily
          font.pixelSize: Style.font.caption
          horizontalAlignment: Text.AlignHCenter
          wrapMode: Text.WordWrap
        }
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
