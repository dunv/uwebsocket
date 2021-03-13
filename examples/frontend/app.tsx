import * as React from "react";
import * as ReactDOM from "react-dom";
import useWebSocket, { ReadyState } from "react-use-websocket";

const rootDiv = document.createElement("div");
rootDiv.id = "root";
document.body.appendChild(rootDiv);

const Index: React.FC<{}> = () => {
  const [input, setInput] = React.useState<string>("");
  const [msgCounter, setMsgCounter] = React.useState<number>(0);
  const [wsError, setWsError] = React.useState<string>();

  const { sendMessage, lastJsonMessage, readyState } = useWebSocket(
    "ws://localhost:8080/wsText",
    {
      onOpen: (e) => {
        setMsgCounter(0);
        console.log("opened", e);
      },
      onClose: (e) => console.log("closed", e),
      onMessage: (e) => {
        setMsgCounter(msgCounter + 1);
        console.log("receivedMessage", e);
      },
      shouldReconnect: (closeEvent) => {
        console.log("closeEvent", closeEvent);
        return true;
      },
      onReconnectStop: (e) => setWsError("reconnects exceeded"),
      retryOnError: true,
    }
  );

  const styleSans: React.CSSProperties = {
    fontFamily: "monospace",
    marginBottom: "1rem",
    whiteSpace: "pre",
  };

  return (
    <div>
      {readyState === ReadyState.CLOSED && <div style={styleSans}>closed</div>}
      {readyState === ReadyState.CLOSING && (
        <div style={styleSans}>closing</div>
      )}
      {readyState === ReadyState.CONNECTING && (
        <div style={styleSans}>connecting</div>
      )}
      {readyState === ReadyState.OPEN && <div style={styleSans}>open</div>}
      {readyState === ReadyState.UNINSTANTIATED && (
        <div style={styleSans}>uninstantiated</div>
      )}
      {readyState === ReadyState.OPEN && (
        <form>
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.currentTarget.value)}
          />
          <button
            onClick={(e) => {
              e.preventDefault();
              input && sendMessage(input);
              setInput("");
            }}
            type="submit"
          >
            Send
          </button>
        </form>
      )}
      {wsError && <div>Err: {JSON.stringify(wsError)}</div>}
      <div>Received messages: {msgCounter}</div>
      <div style={styleSans}>{JSON.stringify(lastJsonMessage, null, "  ")}</div>
    </div>
  );
};

ReactDOM.render(<Index />, document.getElementById("root"));
