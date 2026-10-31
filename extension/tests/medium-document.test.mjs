import assert from 'node:assert/strict';
import test from 'node:test';
import {readMediumEditorDocument} from '../medium-document.js';
const id='fb20c187d357';
const node=(tagName,textContent)=>({tagName,textContent,innerText:textContent});
function fixture(nodes){globalThis.location={origin:'https://medium.com',pathname:`/p/${id}/edit`};globalThis.document={body:{innerText:''},querySelector(selector){return selector.startsWith('.postArticle-content')?{querySelectorAll:()=>nodes}:{textContent:'Saved'}}};}
test('image insertion empty paragraphs are ignored for content verification but included in replacement counts',()=>{
 const nodes=[node('H3','Title'),node('P','Body'),node('FIGURE',''),node('P',''),node('P','\u200b')];fixture(nodes);
 const expected=[{type:3,text:'Title'},{type:1,text:'Body'},{type:1,text:'[BLOGCTL_IMAGE_001]'}];
 assert.equal(readMediumEditorDocument(id,expected).inserted,true);
 assert.equal(readMediumEditorDocument(id).imageCount,5);
 nodes.push(node('P','obsolete footer'));
 assert.match(readMediumEditorDocument(id,expected).error,/paragraphs/);
});
test('verification still detects text loss and code indentation damage',()=>{
 const expected=[{type:3,text:'Title'},{type:8,text:'a\n  b'}];
 fixture([node('H3','Title'),node('PRE','a\n b')]);assert.match(readMediumEditorDocument(id,expected).error,/differs/);
 fixture([node('H3','Title'),node('PRE','a\n  b')]);assert.equal(readMediumEditorDocument(id,expected).inserted,true);
});
