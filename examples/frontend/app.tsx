import * as React from "react";
import * as ReactDOM from "react-dom";
import { ws } from "./websocket";

const rootDiv = document.createElement("div");
rootDiv.id = "root";
document.body.appendChild(rootDiv);

const Index: React.FC<{}> = () => {
  const [wsCloser, setWsCloser] = React.useState<Promise<() => void>>();

  const onMessage = (message: string) => {
    console.log("received message", message);
  };

  const connectSocket = async () => {
    if (!wsCloser) {
      console.log("connecting");
      const closer = ws("ws://localhost:8080", "ws", "example", {}, onMessage);
      setWsCloser(closer);
      console.log("connected");
    }
  };

  const disconnectSocket = async () => {
    if (wsCloser) {
      console.log("disconnecting");
      (await wsCloser)();
      setWsCloser(undefined);
      console.log("disconnected");
    }
  };
  return (
    <div>
      {!wsCloser && <button onClick={() => connectSocket()}>Connect</button>}
      {wsCloser && (
        <button onClick={() => disconnectSocket()}>Disconnect</button>
      )}
    </div>
  );
};

ReactDOM.render(<Index />, document.getElementById("root"));
