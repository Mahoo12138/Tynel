// Only the chosen branch runs. Predicates are native booleans.
function positive(x: number): boolean {
  return x > 0;
}

function choose(x: number): number {
  if (positive(x)) {
    const answer = x + 2;
    return answer;
  } else {
    return 0;
  }
}

console.log(choose(40));
console.log(choose(-1));
console.log(positive(0));
console.log(!positive(-1));
