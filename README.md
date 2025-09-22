# svcinit
[![GoDoc](https://godoc.org/github.com/rrgmc/svcinit?status.png)](https://godoc.org/github.com/rrgmc/svcinit)

## Features

- ordered and unordered stop callbacks.
- stop tasks can work with or without context cancellation.
- ensures no race condition if any starting job returns before all jobs initialized.
- checks all added tasks are properly initialized and all ordered stop tasks are set, ensuring all created tasks always executes. 

## Example

## Author

Rangel Reale (rangelreale@gmail.com)
