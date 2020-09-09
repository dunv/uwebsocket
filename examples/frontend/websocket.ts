import { v4 as uuidv4 } from "uuid";

const backoffInitial = 300;
const backoffLimit = 5000;
let backoffCurrent = backoffInitial;

const increaseBackoff = () => {
  if (backoffCurrent * 2 < backoffLimit) {
    backoffCurrent = backoffCurrent * 2;
  }
};

const resetBackoff = () => {
  backoffCurrent = 300;
};

const connectWs = async (
  url: string,
  name: string,
  onMessage: (msg: any) => void,
  onReconnecting: (evt: CloseEvent) => void,
  onConnected: (evt: Event) => void
): Promise<() => void> => {
  const connUUID = uuidv4();

  // Will hold connection
  let conn: WebSocket | undefined = undefined;

  // Will hold connect-fn
  let connect: () => void = () => undefined;

  // Handlers (to be attached to every new connection)
  let onOpenHandler: (evt: Event) => void = () => undefined;
  let onMessageHandler: (evt: MessageEvent) => void = () => undefined;
  let onErrorHandler: (evt: Event) => void = () => undefined;
  let onCloseHandler: (evt: CloseEvent) => void = () => undefined;

  // If this is set: break reconnect cycle
  let closeRequested = false;

  // Client-side PING-PONG
  const pingPongInterval = 5000;
  const pingPongTimeout = 2000;
  let waitForPingTimeout: NodeJS.Timeout | undefined = undefined;
  let waitForPongTimeout: NodeJS.Timeout | undefined = undefined;

  let reconnectTimeout: NodeJS.Timeout | undefined;

  // Workaround for undetected server-side connection failures
  const terminateFallbackTimeout = 5000;
  let terminateConnectionFallbackTimeout:
    | NodeJS.Timeout
    | undefined = undefined;

  onCloseHandler = (evt: CloseEvent) => {
    conn?.removeEventListener("open", onOpenHandler);
    conn?.removeEventListener("error", onErrorHandler);
    conn?.removeEventListener("close", onCloseHandler);
    conn?.removeEventListener("message", onMessageHandler);
    conn = undefined;
    terminateConnectionFallbackTimeout &&
      clearTimeout(terminateConnectionFallbackTimeout);

    if (!closeRequested) {
      console.log(
        `[websocket] ${name} ${connUUID} RECONNECTING waiting for ${backoffCurrent}`
      );
      if (typeof onReconnecting === "function") onReconnecting(evt);

      reconnectTimeout = setTimeout(() => {
        reconnectTimeout = undefined;
        connect();
      }, backoffCurrent);
    } else {
      console.log(`[websocket] ${name} ${connUUID} CLOSED`);
    }
  };

  // Will take care of client-side ping-pong
  // - send PING
  // - start a new timer, which will wait for the next PONG
  const pingPong = () => {
    // Clear existing timeouts
    waitForPongTimeout && clearTimeout(waitForPongTimeout);
    waitForPingTimeout && clearTimeout(waitForPingTimeout);

    // Start new ping-pong
    waitForPingTimeout = setTimeout(() => {
      conn && conn.send("PING");
      // console.log(`[websocket] ${name} PING`);
      waitForPongTimeout = setTimeout(() => {
        console.log(`[websocket] ${name} ${connUUID} PING_PONG_TIMEOUT`);
        conn?.close(1000, "PING_PONG_TIMEOUT");
        onCloseHandler(new CloseEvent("PING_PONG_TIMEOUT"));
      }, pingPongTimeout);
    }, pingPongInterval);
  };

  onOpenHandler = (evt: Event) => {
    console.log(`[websocket] ${name} ${connUUID} OPENED`);
    resetBackoff();
    pingPong();

    if (typeof onConnected === "function") onConnected(evt);
  };

  onMessageHandler = (evt: MessageEvent) => {
    const messages = evt.data.split("\n");

    // If the message is a PONG message
    // - clear any existing timeout (which would forcefully close the connection when reached)
    // - trigger new PING-PONG
    if (messages.length === 1 && messages[0] === "PONG") {
      // console.log(`[websocket] ${name} PONG`);
      pingPong();
      return;
    }

    for (let i = 0; i < messages.length; i++) {
      if (onMessage) {
        try {
          const parsed = JSON.parse(messages[i]);
          onMessage(parsed);
        } catch (error) {
          console.log(
            `[websocket] ${name} ${connUUID} ERROR could not parse message`,
            messages[i],
            error
          );
        }
      }
    }
  };

  onErrorHandler = (evt: Event) => {
    evt.preventDefault();
    console.log(`[websocket] ${name} ${connUUID} ERROR`);
    increaseBackoff();
  };

  connect = () => {
    if (conn === undefined) {
      conn = new WebSocket(url);
      conn.addEventListener("open", onOpenHandler);
      conn.addEventListener("error", onErrorHandler);
      conn.addEventListener("close", onCloseHandler);
      conn.addEventListener("message", onMessageHandler);
    } else {
      console.log(
        `[websocket] ${name} ${connUUID} Called connect with ongoing connection`
      );
    }
  };

  // Initial connect-call for the WS
  connect();

  // Return fn so the caller can break the reconnect loop
  return () => {
    if (conn) {
      closeRequested = true;
      reconnectTimeout && clearTimeout(reconnectTimeout);
      waitForPingTimeout && clearTimeout(waitForPingTimeout);
      waitForPongTimeout && clearTimeout(waitForPongTimeout);
      console.log(`[websocket] ${name} ${connUUID} CLOSE_REQUESTED`);
      conn.close(1000, "CLOSE_REQUESTED");

      // Fallback, if close event is not received (this happens when connection is cancelled + network connection is lost)
      terminateConnectionFallbackTimeout = setTimeout(() => {
        console.log(`[websocket] ${name} ${connUUID} CLOSE_REQUESTED_FALLBACK`);
        onCloseHandler(new CloseEvent("CLOSE_REQUESTED_FALLBACK"));
      }, terminateFallbackTimeout);
    }
  };
};

export const ws = async (
  url: string,
  path: string,
  name: string,
  config: { [key: string]: string } = {},
  onMessageHandler: (msg: any) => void = () => undefined,
  onReconnecting: (evt: CloseEvent) => void = () => undefined,
  onConnected: (evt: Event) => void = () => undefined
) => {
  const connectParams: { [key: string]: string } = {};
  const paramString = Object.keys(connectParams)
    .map(
      (paramKey) => `${paramKey}=${encodeURIComponent(connectParams[paramKey])}`
    )
    .join("&");

  // console.log(`${url}/${path}?${paramString}`);

  return connectWs(
    `${url}/${path}?${paramString}`,
    name,
    onMessageHandler,
    onReconnecting,
    onConnected
  );
};
