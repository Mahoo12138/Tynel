function mark(id: number, value: boolean): boolean {
  console.log(id);
  return value;
}

console.log(mark(1, true) || mark(2, false) && mark(3, false));
console.log((mark(1, true) || mark(2, false)) && mark(3, false));
