export class TaronjaGatewayError extends Error {
    public readonly status?: number;
    public readonly data?: unknown;

    constructor(message: string, status?: number, data?: unknown) {
        super(message);
        this.name = 'TaronjaGatewayError';
        this.status = status;
        this.data = data;
    }
}
