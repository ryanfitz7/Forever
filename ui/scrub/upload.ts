// Sends a beta client file to the collector, so that contributing is a drag and a click
// rather than a GitHub account, an issue template and an attachment.
//
// The endpoint only accepts two shapes and only from this origin; see
// infra/cloudflare/uploads/src/index.js for what it does with them. Nothing here retries,
// because a failed upload should say so and offer the file back rather than quietly
// trying again with bytes the sender has stopped watching.

export const UPLOAD_URL = import.meta.env.VITE_UPLOAD_URL || '';

export type UploadKind = 'dbcache' | 'damagemeter' | 'screenshot';

export type UploadResult = { ok: true; receipt: string } | { ok: false; error: string };

export async function upload(kind: UploadKind, bytes: Uint8Array, note: string): Promise<UploadResult> {
	if (!UPLOAD_URL) return { ok: false, error: 'Uploads are not configured for this site. Save the file locally instead.' };
	const url = `${UPLOAD_URL}?kind=${kind}&note=${encodeURIComponent(note.slice(0, 200))}`;
	try {
		const response = await fetch(url, {
			method: 'POST',
			headers: { 'Content-Type': 'application/octet-stream' },
			body: bytes as unknown as BodyInit,
		});
		const body = (await response.json()) as { ok?: boolean; receipt?: string; error?: string };
		if (response.ok && body.ok && body.receipt) return { ok: true, receipt: body.receipt };
		return { ok: false, error: body.error || `upload failed (${response.status})` };
	} catch (error) {
		// A network failure, an extension blocking the request, an offline tab.
		return { ok: false, error: String(error) };
	}
}
