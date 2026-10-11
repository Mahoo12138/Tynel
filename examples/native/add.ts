// This source is compiled by tsgo, then lowered to Tynel NIR.
function add(a: number, b: number): number {
  return a + b;
}
const answer: number = add(40, 2);
console.log(answer);
