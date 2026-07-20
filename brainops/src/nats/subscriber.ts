// NATS connection + subscription with reconnect logic

import { connect, type NatsConnection, type Subscription } from "nats";
import { parseIncidentMessage, type NatsIncidentMessage } from "./types.js";

export class NatsSubscriber {
  private connection: NatsConnection | null = null;
  private subscriptions: Subscription[] = [];
  private connected = false;
  private subject: string;

  constructor(subject: string) {
    this.subject = subject;
  }

  /**
   * Connects to a NATS server with reconnect logic.
   */
  async connect(url: string): Promise<void> {
    this.connection = await connect({
      servers: url,
      maxReconnectAttempts: 10,
      reconnectTimeWait: 2000,
    });

    this.connected = true;

    // Monitor connection status events
    void this.monitorStatus();
  }

  /**
   * Subscribes to a subject and deserializes JSON messages before passing to handler.
   */
  subscribe(subject: string, handler: (data: unknown) => Promise<void>): void {
    if (!this.connection) {
      throw new Error("NatsSubscriber: not connected. Call connect() first.");
    }

    const sub = this.connection.subscribe(subject);
    this.subscriptions.push(sub);

    void (async () => {
      for await (const msg of sub) {
        try {
          const decoded = new TextDecoder().decode(msg.data);
          const data: unknown = JSON.parse(decoded);
          await handler(data);
        } catch (err) {
          console.warn(
            `[NatsSubscriber] Error processing message on "${subject}":`,
            err instanceof Error ? err.message : err,
          );
        }
      }
    })();
  }

  /**
   * Subscribes to the configured incident subject, parses messages as
   * NatsIncidentMessage, and passes valid incidents to the handler.
   */
  subscribeToIncidents(handler: (incident: NatsIncidentMessage) => Promise<void>): void {
    this.subscribe(this.subject, async (data: unknown) => {
      const incident = parseIncidentMessage(data);
      await handler(incident);
    });
  }

  /**
   * Returns whether the subscriber is currently connected to NATS.
   */
  isConnected(): boolean {
    return this.connected;
  }

  /**
   * Drains all subscriptions and closes the connection.
   */
  async close(): Promise<void> {
    if (this.connection) {
      await this.connection.drain();
      this.connected = false;
      this.connection = null;
      this.subscriptions = [];
    }
  }

  /**
   * Monitors NATS connection status events (disconnect/reconnect).
   */
  private async monitorStatus(): Promise<void> {
    if (!this.connection) return;

    for await (const status of this.connection.status()) {
      switch (status.type) {
        case "disconnect":
          this.connected = false;
          console.warn("[NatsSubscriber] Disconnected from NATS server");
          break;
        case "reconnect":
          this.connected = true;
          console.info("[NatsSubscriber] Reconnected to NATS server");
          break;
        case "error":
          console.warn("[NatsSubscriber] Connection error:", status.data);
          break;
      }
    }
  }
}
