import test from 'node:test';
import assert from 'node:assert/strict';
import { read51CTOInventoryInPage } from '../cto51-inventory.js';

async function withinPage(fakeFetch, action, origin = 'https://blog.51cto.com') {
  const originalFetch = globalThis.fetch;
  const originalLocation = globalThis.location;
  try {
    globalThis.location = { origin };
    globalThis.fetch = fakeFetch;
    return await action();
  } finally {
    if (originalLocation === undefined) delete globalThis.location;
    else globalThis.location = originalLocation;
    globalThis.fetch = originalFetch;
  }
}

test('read-only 51CTO inventory uses exactly the first-party endpoint for drafts and published', async () => {
  const calls = [];
  const fakeFetch = async (url, init) => {
    calls.push({ url, method: init.method, credentials: init.credentials, data: JSON.parse(init.body.get('data')) });
    const published = calls.length === 2;
    return {
      ok: true, headers: new Headers({ 'content-type': 'application/json;charset=utf-8' }),
      json: async () => ({ status: 1, data: { list: [{ blog_id: published ? 20 : 10, title: published ? 'Published' : 'Draft', blog_url: '/u_1/20' }], pagination: '' } }),
    };
  };
  const result = await withinPage(fakeFetch, () => read51CTOInventoryInPage());
  assert.deepEqual(result.items.map((x) => [x.id, x.published]), [['10', false], ['20', true]]);
  assert.deepEqual(calls.map((x) => x.data.type), [4, 2]);
  for (const call of calls) {
    assert.equal(call.url, 'https://blog.51cto.com/creative-center-ajax/list');
    assert.equal(call.method, 'POST');
    assert.equal(call.credentials, 'same-origin');
  }
});

test('rejects a wrong origin without issuing any request', async () => {
  await assert.rejects(withinPage(() => { throw new Error('should not fetch'); }, read51CTOInventoryInPage, 'https://example.org'), /first-party|signed-in/);
});

test('fails safely on HTML response; never tries to interpret login or challenge HTML as JSON', async () => {
  const mock = async () => ({ ok: true, headers: new Headers({ 'content-type': 'text/html' }) });
  await assert.rejects(withinPage(mock, read51CTOInventoryInPage), /unexpected page/);
});

test('forbids off-platform edit URLs', async () => {
  let call = 0;
  const mock = async () => {
    call++;
    return { ok: true, headers: new Headers({ 'content-type': 'application/json' }), json: async () => ({
      status: 1, data: { list: call === 1 ? [{ blog_id: 8, title: 'x', edit_url: 'https://evil.invalid/private' }] : [], pagination: '' },
    }) };
  };
  const result = await withinPage(mock, read51CTOInventoryInPage);
  assert.equal(result.items[0].url, 'https://blog.51cto.com/blogger/draft/8');
});
